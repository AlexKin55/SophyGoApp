import type { MicVAD as MicVADInstance } from '@ricky0123/vad-web'
import { env } from '../config/env'
import { logger } from '../config/logger'

/**
 * Silero VAD работает на устройстве через ONNX Runtime Web (WASM).
 *
 * По умолчанию модель и wasm отдаются с того же Origin из /vad/ и /ort/
 * (см. `npm run vad:assets`). Это надёжнее CDN внутри Telegram WebView.
 * Переопределить пути можно через VITE_VAD_ASSET_BASE_URL и
 * VITE_ORT_ASSET_BASE_URL; оба значения должны заканчиваться на "/".
 */
/**
 * SileroVadGate — обёртка над MicVAD.
 *
 * Использует тот же MediaStream, что и RealtimeAudio, поэтому микрофон
 * открывается один раз. Пока модель загружается, GuideClient использует
 * энергетический VAD по RMS.
 */
export class SileroVadGate {
  private vad: MicVADInstance | null = null
  private ready = false
  private failed = false
  private speaking = false
  private lastSpeechAt = Date.now()
  private lastFrameAt = 0

  /** Готов ли VAD и продолжает ли он получать аудиокадры. */
  isReady(): boolean {
    return this.ready && Date.now() - this.lastFrameAt < env.vad.frameStaleMs
  }

  hasFailed(): boolean {
    return this.failed
  }

  isSpeaking(): boolean {
    return this.isReady() && this.speaking
  }

  /** Сколько миллисекунд прошло с последней речи. */
  silenceMs(): number {
    return Date.now() - this.lastSpeechAt
  }

  /** Загружает модель и запускает VAD на уже открытом потоке. */
  async start(stream: MediaStream): Promise<boolean> {
    if (this.vad) return true
    if (this.failed) return false

    try {
      const { MicVAD } = await import('@ricky0123/vad-web')

      const vad = await MicVAD.new({
        getStream: async () => stream,
        model: 'v5',
        baseAssetPath: env.vadAssetBaseUrl,
        onnxWASMBasePath: env.ortAssetBaseUrl,
        processorType: 'auto',
        startOnLoad: false,
        positiveSpeechThreshold: env.vad.speechThreshold,
        negativeSpeechThreshold: env.vad.negativeThreshold,
        redemptionMs: env.vad.redemptionMs,
        preSpeechPadMs: env.vad.preSpeechPadMs,
        minSpeechMs: env.vad.minSpeechMs,
        ortConfig: (ort) => {
          ort.env.logLevel = 'error'
          // Один поток надёжнее в WebView без cross-origin isolation
          // и достаточен для Silero v5 на кадрах по 32 мс.
          ort.env.wasm.numThreads = env.vad.numThreads
          ort.env.wasm.simd = env.vad.simd
        },
        onSpeechStart: () => {
          this.markSpeech()
        },
        onSpeechRealStart: () => {
          this.markSpeech()
        },
        onSpeechEnd: () => {
          this.speaking = false
          this.lastSpeechAt = Date.now()
          this.lastFrameAt = Date.now()
        },
        onVADMisfire: () => {
          this.speaking = false
        },
        onFrameProcessed: (probabilities) => {
          this.lastFrameAt = Date.now()
          if (probabilities.isSpeech >= env.vad.speechThreshold) {
            this.markSpeech()
          }
        },
      })

      await vad.start()
      this.vad = vad
      this.ready = true
      this.lastFrameAt = Date.now()
      return true
    } catch (error) {
      this.failed = true
      this.ready = false
      logger.warn('Silero VAD недоступен, используется энергетический VAD', { error: String(error) })
      return false
    }
  }

  /** Останавливает VAD и освобождает модель. */
  async stop(): Promise<void> {
    const vad = this.vad
    this.vad = null
    this.ready = false
    this.speaking = false
    this.lastFrameAt = 0

    if (vad) {
      try {
        await vad.destroy()
      } catch {
        // VAD уже остановлен.
      }
    }
  }

  private markSpeech(): void {
    this.speaking = true
    this.lastSpeechAt = Date.now()
    this.lastFrameAt = Date.now()
  }
}
