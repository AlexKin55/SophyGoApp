import { useCallback, useEffect, useRef, useState } from 'react'
import { GuideClient, initialGuideState, type GuideState } from './guideClient'
import { getTelegramInitData, initTelegram, loadTelegramSdk } from '../platform/telegram'
import { isDevAuthEnabled } from '../platform/devAuth'
import type { Place } from '../places/places'

export interface GuideController extends GuideState {
  handlePlaceOpen: (place: Place) => Promise<void>
  toggleListening: () => Promise<void>
  stop: () => Promise<void>
}

/** React-обёртка над GuideClient. */
export function useGuideSession(): GuideController {
  const clientRef = useRef<GuideClient | null>(null)
  const [state, setState] = useState<GuideState>(initialGuideState)

  useEffect(() => {
    const client = new GuideClient()
    clientRef.current = client
    const unsubscribe = client.subscribe(setState)
    let cancelled = false

    const devAuth = isDevAuthEnabled()

    void (async () => {
      // В обычном браузере SDK Telegram не нужен и может быть недоступен.
      if (!devAuth) {
        await loadTelegramSdk()
      }
      if (cancelled) return

      initTelegram()

      // В Telegram сразу создаём сессию. В обычном браузере подключаемся
      // только при включённом dev-режиме (VITE_DEV_AUTH=true).
      if (getTelegramInitData() || devAuth) {
        void client.connect()
      }
    })()

    return () => {
      cancelled = true
      unsubscribe()
      client.dispose()
      clientRef.current = null
    }
  }, [])

  const handlePlaceOpen = useCallback(async (place: Place) => {
    await clientRef.current?.handlePlaceOpen(place)
  }, [])

  const toggleListening = useCallback(async () => {
    await clientRef.current?.toggleListening()
  }, [])

  const stop = useCallback(async () => {
    await clientRef.current?.stop()
  }, [])

  return {
    ...state,
    handlePlaceOpen,
    toggleListening,
    stop,
  }
}
