/**
 * Захват и воспроизведение PCM16 mono для Yandex Realtime API.
 *
 * Частоты берутся из ответа POST /api/v1/session (inputRate/outputRate).
 * По умолчанию 16000 Гц — wideband-речь с минимальным трафиком.
 * Браузерный AudioContext может работать на другой частоте, поэтому
 * выполняется линейный ресемплинг.
 */
import { env } from '../config/env'

export class RealtimeAudio {
  private inputRate = env.audio.defaultSampleRate
  private outputRate = env.audio.defaultSampleRate

  private playbackContext: AudioContext | null = null
  private activeSources = new Set<AudioBufferSourceNode>()
  private nextStartTime = 0

  private captureContext: AudioContext | null = null
  private captureStream: MediaStream | null = null
  private captureSource: MediaStreamAudioSourceNode | null = null
  private processor: ScriptProcessorNode | null = null
  private silentGain: GainNode | null = null
  private chunkHandler: ((pcm: Uint8Array, rms: number) => void) | null = null

  /** Устанавливает частоты из сессии Yandex Realtime. */
  setRates(inputRate?: number, outputRate?: number): void {
    if (inputRate && inputRate > 0) {
      this.inputRate = inputRate
    }
    if (outputRate && outputRate > 0) {
      this.outputRate = outputRate
    }
  }

  /** Создаёт/возобновляет AudioContext. Вызывать из пользовательского жеста. */
  async ensurePlayback(): Promise<void> {
    if (!this.playbackContext) {
      this.playbackContext = createAudioContext(this.outputRate)
    }
    if (this.playbackContext.state === 'suspended') {
      await this.playbackContext.resume()
    }
  }

  /** Проигрывает фрагмент ответа Гида (base64 PCM16). */
  async playBase64Pcm(encoded: string): Promise<void> {
    const bytes = decodeBase64(encoded)
    if (bytes.length < 2) return

    await this.ensurePlayback()
    const context = this.playbackContext
    if (!context) return

    const pcm = new Int16Array(bytes.buffer, bytes.byteOffset, Math.floor(bytes.byteLength / 2))
    const samples = new Float32Array(pcm.length)
    for (let i = 0; i < pcm.length; i += 1) {
      samples[i] = pcm[i] / 32768
    }

    const resampled = resampleLinear(samples, this.outputRate, context.sampleRate)
    if (resampled.length === 0) return

    const buffer = context.createBuffer(1, resampled.length, context.sampleRate)
    buffer.getChannelData(0).set(resampled)

    const source = context.createBufferSource()
    source.buffer = buffer
    source.connect(context.destination)

    const startAt = Math.max(context.currentTime, this.nextStartTime)
    source.start(startAt)
    this.nextStartTime = startAt + buffer.duration

    this.activeSources.add(source)
    source.onended = () => {
      this.activeSources.delete(source)
    }
  }

  /** Мгновенно останавливает воспроизведение (перебивание). */
  stopPlayback(): void {
    for (const source of this.activeSources) {
      try {
        source.stop()
      } catch {
        // Источник уже остановлен.
      }
    }
    this.activeSources.clear()
    this.nextStartTime = this.playbackContext?.currentTime ?? 0
  }

  /** Возвращает активный поток микрофона (для Silero VAD). */
  getStream(): MediaStream | null {
    return this.captureStream
  }

  /** Начинает захват микрофона и отправляет PCM16-чанки и их RMS в callback. */
  async startCapture(onChunk: (pcm: Uint8Array, rms: number) => void): Promise<void> {
    this.chunkHandler = onChunk

    const streamAlive =
      this.captureStream?.getAudioTracks().some((track) => track.readyState === 'live') ?? false

    if (!streamAlive) {
      this.releaseCapture()
      if (!navigator.mediaDevices?.getUserMedia) {
        throw new Error('Микрофон недоступен в этом браузере')
      }

      // Разрешение спрашиваем один раз за сессию: поток сохраняется и
      // переиспользуется при повторном включении микрофона.
      this.captureStream = await navigator.mediaDevices.getUserMedia({
        audio: {
          channelCount: 1,
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
      })
      this.captureContext = createAudioContext(this.inputRate)
    }

    const context = this.captureContext
    const stream = this.captureStream
    if (!context || !stream) {
      throw new Error('Не удалось получить доступ к микрофону')
    }

    if (context.state === 'suspended') {
      await context.resume()
    }
    if (this.processor) return

    this.captureSource = context.createMediaStreamSource(stream)
    this.processor = context.createScriptProcessor(env.audio.captureBufferSize, 1, 1)
    this.silentGain = context.createGain()
    this.silentGain.gain.value = 0

    this.processor.onaudioprocess = (event) => {
      const input = event.inputBuffer.getChannelData(0)
      const resampled = resampleLinear(input, context.sampleRate, this.inputRate)
      const pcm = new Int16Array(resampled.length)

      let sumSquares = 0
      for (let i = 0; i < resampled.length; i += 1) {
        const sample = Math.max(-1, Math.min(1, resampled[i]))
        pcm[i] = sample < 0 ? sample * 0x8000 : sample * 0x7fff
        sumSquares += sample * sample
      }

      const rms = resampled.length > 0 ? Math.sqrt(sumSquares / resampled.length) : 0
      this.chunkHandler?.(new Uint8Array(pcm.buffer), rms)
    }

    this.captureSource.connect(this.processor)
    this.processor.connect(this.silentGain)
    this.silentGain.connect(context.destination)
  }

  /** Отключает отправку чанков, но оставляет микрофон открытым. */
  pauseCapture(): void {
    this.chunkHandler = null

    this.processor?.disconnect()
    this.processor = null
    this.captureSource?.disconnect()
    this.captureSource = null
    this.silentGain?.disconnect()
    this.silentGain = null
  }

  /** Останавливает микрофон и освобождает устройства. */
  stopCapture(): void {
    this.pauseCapture()
    this.releaseCapture()
  }

  private releaseCapture(): void {
    this.captureStream?.getTracks().forEach((track) => track.stop())
    this.captureStream = null

    if (this.captureContext) {
      void this.captureContext.close()
      this.captureContext = null
    }
  }

  /** Полностью освобождает аудиоресурсы. */
  dispose(): void {
    this.stopPlayback()
    this.stopCapture()
    if (this.playbackContext) {
      void this.playbackContext.close()
      this.playbackContext = null
    }
  }
}

function createAudioContext(preferredRate?: number): AudioContext {
  try {
    return new AudioContext(preferredRate ? { sampleRate: preferredRate } : undefined)
  } catch {
    return new AudioContext()
  }
}

function resampleLinear(input: Float32Array, fromRate: number, toRate: number): Float32Array {
  if (input.length === 0 || fromRate === toRate) return input

  const ratio = fromRate / toRate
  const length = Math.max(1, Math.floor(input.length / ratio))
  const output = new Float32Array(length)

  for (let i = 0; i < length; i += 1) {
    const position = i * ratio
    const index = Math.floor(position)
    const fraction = position - index
    const next = Math.min(index + 1, input.length - 1)
    output[i] = input[index] * (1 - fraction) + input[next] * fraction
  }

  return output
}

function decodeBase64(encoded: string): Uint8Array {
  const binary = atob(encoded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

export function encodeBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunkSize = env.audio.base64ChunkSize
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}
