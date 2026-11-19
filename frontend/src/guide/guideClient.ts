import { RealtimeAudio, encodeBase64 } from '../audio/audioEngine'
import { SileroVadGate } from '../audio/sileroVad'
import { buildRealtimeUrl, createGuideSession, stopGuide, type GuideSession } from '../api/guide'
import { sendPlaceOpen } from '../api/events'
import { getTelegramInitData } from '../platform/telegram'
import { isDevAuthEnabled } from '../platform/devAuth'
import { env } from '../config/env'
import { logger } from '../config/logger'
import type { Place } from '../places/places'

export type GuideStatus =
  | 'idle'
  | 'connecting'
  | 'ready'
  | 'listening'
  | 'speaking'
  | 'error'
  | 'unavailable'

export type VadStatus = 'idle' | 'loading' | 'ready' | 'fallback'

export interface GuideState {
  status: GuideStatus
  connected: boolean
  listening: boolean
  vadStatus: VadStatus
  error: string | null
  transcript: string
  userTranscript: string
}

export const initialGuideState: GuideState = {
  status: 'idle',
  connected: false,
  listening: false,
  vadStatus: 'idle',
  error: null,
  transcript: '',
  userTranscript: '',
}

interface ServerEvent {
  type?: string
  delta?: string
  transcript?: string
  error?: {
    message?: string
  }
}

type Listener = (state: GuideState) => void

/**
 * GuideClient управляет жизненным циклом голосовой сессии:
 * создаёт серверную сессию, открывает WebSocket, захватывает микрофон,
 * проигрывает PCM-ответы Гида и выключает микрофон при длительной тишине.
 *
 * Детекция речи: Silero VAD (ONNX/WASM) на устройстве, с энергетическим
 * RMS-fallback на время загрузки модели.
 */
export class GuideClient {
  private readonly audio = new RealtimeAudio()
  private readonly listeners = new Set<Listener>()

  private state: GuideState = initialGuideState
  private ws: WebSocket | null = null
  private session: GuideSession | null = null
  private connectPromise: Promise<void> | null = null
  private listening = false
  private disposed = false
  private listenGeneration = 0

  // Эпохи защищают от «хвостов» прошлого ответа после перебивания.
  private playbackEpoch = 0
  private responseEpoch: number | null = null

  // VAD и контроль тишины.
  private sileroVad: SileroVadGate | null = null
  private idleTimer: number | null = null
  private lastVoiceAt = 0
  private voiceFrames = 0

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener)
    listener(this.state)
    return () => {
      this.listeners.delete(listener)
    }
  }

  getState(): GuideState {
    return this.state
  }

  /** Создаёт сессию и открывает WebSocket. */
  async connect(): Promise<void> {
    if (this.disposed) return
    if (this.ws?.readyState === WebSocket.OPEN) return
    if (this.connectPromise) return this.connectPromise

    logger.debug('guide connect requested')

    // В обычном браузере initData нет: разрешаем подключение только при
    // явно включённом dev-режиме (VITE_DEV_AUTH=true на frontend и
    // DEV_AUTH_USER_ID на backend).
    if (!getTelegramInitData() && !isDevAuthEnabled()) {
      this.update({
        status: 'unavailable',
        error: 'Голосовой Гид доступен только в Telegram',
      })
      return
    }

    this.update({ status: 'connecting', error: null })
    this.connectPromise = this.openConnection()

    try {
      await this.connectPromise
    } catch (error) {
      logger.error('guide connect failed', { error: errorMessage(error) })
      this.update({ status: 'error', connected: false, error: errorMessage(error) })
    } finally {
      this.connectPromise = null
    }
  }

  /** Отправляет объект на сервер, чтобы Гид начал рассказ. */
  async handlePlaceOpen(place: Place): Promise<void> {
    logger.info('place open requested', { placeId: place.id, placeName: place.name })
    try {
      await this.audio.ensurePlayback()

      // Клик по объекту — пользовательский жест: сначала запрашиваем микрофон,
      // пока браузер не потерял контекст жеста, затем подключаемся к серверу.
      if (!this.listening) {
        await this.startListening()
      }
      await this.connect()

      // Новый объект обрабатываем как перебивание: Гид прерывает текущий
      // рассказ и сразу начинает говорить про выбранное место.
      this.handleInterruption(true)
      this.update({ transcript: '', error: null })

      const result = await sendPlaceOpen(place, this.session?.sessionId)
      if (result?.sessionId && this.session) {
        this.session = { ...this.session, sessionId: result.sessionId }
      }
      if (result && !result.delivered) {
        this.update({ error: 'Гид не получил объект. Попробуйте ещё раз.' })
      }
    } catch (error) {
      logger.error('place open failed', { placeId: place.id, error: errorMessage(error) })
      this.update({ error: errorMessage(error) })
    }
  }

  /** Включает или выключает микрофон. */
  async toggleListening(): Promise<void> {
    logger.debug('toggle listening', { listening: this.listening })
    if (this.listening) {
      this.stopListening()
      return
    }
    await this.startListening()
  }

  /** Останавливает текущий ответ Гида и воспроизведение. */
  async stop(): Promise<void> {
    logger.info('guide stop requested', { sessionId: this.session?.sessionId })
    this.handleInterruption(true)

    if (this.session) {
      void stopGuide(this.session.sessionId).catch(() => {
        // WebSocket уже отправляет response.cancel; HTTP-вызов вспомогательный.
      })
    }
  }

  /**
   * Общее перебивание: сбрасывает эпохи, останавливает звук и при
   * необходимости отменяет текущий ответ. Используется и при выборе
   * нового объекта, и при перебивании голосом.
   */
  private handleInterruption(cancelResponse: boolean): void {
    // response.cancel имеет смысл только при активном ответе: иначе
    // Yandex присылает "Cancellation failed: no active response found".
    const hasActiveResponse = this.responseEpoch !== null || this.state.status === 'speaking'

    this.playbackEpoch += 1
    this.responseEpoch = null
    this.audio.stopPlayback()
    if (cancelResponse && hasActiveResponse) {
      this.send({ type: 'response.cancel' })
    }
    this.update({
      status: this.listening ? 'listening' : this.state.connected ? 'ready' : this.state.status,
    })
  }

  /** Освобождает все ресурсы. */
  dispose(): void {
    this.disposed = true
    this.listeners.clear()

    this.teardownCapture()
    this.audio.dispose()

    if (this.ws) {
      this.ws.onclose = null
      this.ws.onerror = null
      this.ws.onmessage = null
      try {
        this.ws.close()
      } catch {
        // Соединение уже закрыто.
      }
      this.ws = null
    }
  }

  private async startListening(): Promise<void> {
    const generation = ++this.listenGeneration

    try {
      await this.audio.ensurePlayback()
      if (generation !== this.listenGeneration) return

      this.lastVoiceAt = Date.now()
      this.voiceFrames = 0

      // getUserMedia вызываем до сетевых запросов, чтобы не потерять
      // пользовательский жест в Safari и Telegram WebView на iOS.
      await this.audio.startCapture((pcm, rms) => {
        this.handleMicChunk(pcm, rms)
      })
      if (generation !== this.listenGeneration) {
        this.audio.stopCapture()
        return
      }

      // Silero VAD загружается лениво на том же потоке. До готовности модели
      // работает RMS-fallback, поэтому микрофон не блокируется.
      const stream = this.audio.getStream()
      if (stream) {
        const vad = new SileroVadGate()
        this.sileroVad = vad
        this.update({ vadStatus: 'loading' })
        void vad.start(stream).then((ok) => {
          if (this.sileroVad !== vad) return
          this.update({ vadStatus: ok ? 'ready' : 'fallback' })
        })
      } else {
        this.update({ vadStatus: 'fallback' })
      }

      this.listening = true
      this.startIdleWatch()
      this.update({ listening: true, status: 'connecting' })

      await this.connect()

      if (generation !== this.listenGeneration || this.disposed) return
      if (!this.state.connected) {
        this.teardownCapture()
        this.update({ listening: false, status: this.state.status })
        return
      }

      this.update({ listening: true, status: 'listening', error: null })
    } catch (error) {
      if (generation !== this.listenGeneration) return
      this.teardownCapture()
      this.update({ listening: false, status: 'error', error: errorMessage(error) })
    }
  }

  private stopListening(): void {
    this.teardownCapture()

    let status: GuideStatus = 'idle'
    if (this.state.connected) {
      status = 'ready'
    } else if (this.state.status === 'error') {
      status = 'error'
    }

    this.update({ listening: false, status })
  }

  private teardownCapture(): void {
    this.listenGeneration += 1
    this.stopIdleWatch()
    this.audio.stopCapture()

    const vad = this.sileroVad
    this.sileroVad = null
    if (vad) {
      void vad.stop()
    }

    this.listening = false
    this.voiceFrames = 0
    this.update({ vadStatus: 'idle' })
  }

  /**
   * Решает, отправлять ли фрагмент микрофона в Realtime API.
   *
   * Пока Гид говорит, тишина и эхо динамика не отправляются. Голос
   * пользователя отправляется сразу — это обеспечивает перебивание.
   */
  private handleMicChunk(pcm: Uint8Array, rms: number): void {
    const vad = this.sileroVad
    const vadReady = vad?.isReady() ?? false
    const hasVoice = vadReady ? vad!.isSpeaking() : rms >= env.guide.speechRmsThreshold

    if (hasVoice) {
      this.lastVoiceAt = Date.now()
      this.voiceFrames = Math.min(this.voiceFrames + 1, env.guide.bargeInFrames)
    } else {
      this.voiceFrames = Math.max(this.voiceFrames - 1, 0)
    }

    const requiredFrames = vadReady ? 1 : env.guide.bargeInFrames
    if (this.state.status === 'speaking' && this.voiceFrames < requiredFrames) {
      return
    }

    this.send({
      type: 'input_audio_buffer.append',
      audio: encodeBase64(pcm),
    })
  }

  /** Выключает микрофон, если пользователь давно ничего не говорил. */
  private startIdleWatch(): void {
    this.stopIdleWatch()

    this.idleTimer = window.setInterval(() => {
      if (!this.listening || this.state.status === 'speaking') return

      const vad = this.sileroVad
      const silentFor = vad?.isReady() ? vad.silenceMs() : Date.now() - this.lastVoiceAt
      if (silentFor < env.guide.idleTimeoutMs) return

      this.stopListening()
      this.update({
        status: 'ready',
        error: 'Микрофон выключен из-за тишины. Нажмите «Говорить с Гидом», чтобы продолжить.',
      })
    }, env.guide.idleCheckIntervalMs)
  }

  private stopIdleWatch(): void {
    if (this.idleTimer !== null) {
      window.clearInterval(this.idleTimer)
      this.idleTimer = null
    }
  }

  private async openConnection(): Promise<void> {
    const session = await createGuideSession()
    if (this.disposed) return

    logger.info('guide session ready', {
      sessionId: session.sessionId,
      realtimeAvailable: session.realtimeAvailable,
      inputRate: session.inputRate,
      outputRate: session.outputRate,
    })

    this.session = session
    this.audio.setRates(session.inputRate, session.outputRate)

    if (!session.realtimeAvailable) {
      this.update({
        status: 'error',
        error: 'Yandex Realtime API не настроен на сервере',
      })
      return
    }

    const ws = new WebSocket(buildRealtimeUrl(session))
    this.ws = ws

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        this.handleServerEvent(event.data)
      }
    }
    ws.onclose = () => {
      logger.info('guide websocket closed')
      this.handleClose()
    }
    ws.onerror = () => {
      logger.warn('guide websocket error')
      if (!this.disposed) {
        this.update({ error: 'Ошибка соединения с Гидом', status: 'error' })
      }
    }

    try {
      await waitForOpen(ws)
    } catch (error) {
      if (this.ws === ws) this.ws = null
      try {
        ws.close()
      } catch {
        // Соединение уже закрыто.
      }
      throw error
    }

    if (this.disposed) {
      ws.close()
      return
    }

    logger.info('guide websocket connected')
    this.update({ connected: true, status: 'ready', error: null })
  }

  private handleClose(): void {
    this.ws = null
    const wasConnected = this.state.connected

    this.teardownCapture()

    if (this.disposed) return

    this.update({
      connected: false,
      listening: false,
      status: wasConnected ? 'idle' : this.state.status === 'connecting' ? 'error' : this.state.status,
      error: wasConnected ? 'Соединение с Гидом закрыто' : this.state.error,
    })
  }

  private handleServerEvent(raw: string): void {
    let event: ServerEvent
    try {
      event = JSON.parse(raw) as ServerEvent
    } catch {
      return
    }
    logger.debug('guide server event', { type: event.type })

    switch (event.type) {
      case 'response.created':
        this.responseEpoch = this.playbackEpoch
        this.update({ transcript: '', status: 'speaking' })
        break

      case 'response.output_audio.delta':
        if (event.delta && this.responseEpoch === this.playbackEpoch) {
          if (this.state.status !== 'speaking') {
            this.update({ status: 'speaking' })
          }
          void this.audio.playBase64Pcm(event.delta).catch(() => {
            // Повреждённый аудиофрагмент пропускаем.
          })
        }
        break

      case 'response.output_text.delta':
        if (event.delta && this.responseEpoch === this.playbackEpoch) {
          this.update({
            transcript: this.state.transcript + event.delta,
            status: 'speaking',
          })
        }
        break

      case 'input_audio_buffer.speech_started':
        // Пользователь перебил Гида: серверный VAD уже отменяет ответ,
        // поэтому дополнительный response.cancel не отправляем.
        this.handleInterruption(false)
        this.lastVoiceAt = Date.now()
        this.voiceFrames = env.guide.bargeInFrames
        break

      case 'conversation.item.input_audio_transcription.completed':
        if (event.transcript) {
          this.update({ userTranscript: event.transcript })
        }
        break

      case 'response.done':
        if (this.responseEpoch === this.playbackEpoch) {
          this.responseEpoch = null
          this.update({ status: this.listening ? 'listening' : 'ready' })
        }
        break

      case 'error': {
        const message = event.error?.message ?? 'Ошибка Гида'
        if (isCancellationNoise(message)) {
          // Гонка отмены: ответ уже завершился, это не ошибка для пользователя.
          logger.debug('realtime: игнорирую ошибку отмены', { message })
          break
        }
        this.update({ status: 'error', error: message })
        break
      }

      default:
        break
    }
  }

  private send(payload: unknown): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(payload))
    }
  }

  private update(partial: Partial<GuideState>): void {
    if (this.disposed) return
    this.state = { ...this.state, ...partial }
    for (const listener of this.listeners) {
      listener(this.state)
    }
  }
}

function waitForOpen(ws: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    if (ws.readyState === WebSocket.OPEN) {
      resolve()
      return
    }

    const timer = window.setTimeout(() => {
      cleanup()
      reject(new Error('Таймаут подключения к Гиду'))
    }, env.guide.connectTimeoutMs)

    const cleanup = () => {
      window.clearTimeout(timer)
      ws.removeEventListener('open', handleOpen)
      ws.removeEventListener('error', handleError)
      ws.removeEventListener('close', handleClose)
    }

    const handleOpen = () => {
      cleanup()
      resolve()
    }
    const handleError = () => {
      cleanup()
      reject(new Error('Не удалось подключиться к Гиду'))
    }
    const handleClose = () => {
      cleanup()
      reject(new Error('Соединение с Гидом закрыто'))
    }

    ws.addEventListener('open', handleOpen)
    ws.addEventListener('error', handleError)
    ws.addEventListener('close', handleClose)
  })
}

function isCancellationNoise(message: string): boolean {
  const text = message.toLowerCase()
  return text.includes('no active response') || text.includes('cancellation failed')
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    switch (error.name) {
      case 'NotAllowedError':
      case 'PermissionDeniedError':
        return 'Нет доступа к микрофону. Разрешите его в настройках браузера.'
      case 'NotFoundError':
      case 'DevicesNotFoundError':
        return 'Микрофон не найден. Подключите его и попробуйте снова.'
      case 'NotReadableError':
      case 'TrackStartError':
        return 'Микрофон занят другим приложением.'
      default:
        break
    }

    if (error.message) {
      return error.message
    }
  }
  return 'Не удалось запустить голосового Гида'
}
