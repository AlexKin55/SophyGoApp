import { useCallback, useEffect, useState } from 'react'
import { env } from '../config/env'

export interface GeoPosition {
  latitude: number
  longitude: number
  accuracy: number
  timestamp: number
}

export type GeoErrorCode = 'permission-denied' | 'unavailable' | 'timeout' | 'unsupported' | 'unknown'

export interface UseGeolocationReturn {
  position: GeoPosition | null
  error: string | null
  errorCode: GeoErrorCode | null
  isSupported: boolean
  retry: () => void
}

/**
 * Следит за геопозицией пользователя.
 *
 * Геолокация не обязательна для работы приложения: если доступ запрещён,
 * карта остаётся на стартовом центре, а пользователь может двигать её вручную.
 * `retry` повторяет запрос — полезно после того, как доступ включили в
 * настройках Telegram или браузера.
 */
export function useGeolocation(): UseGeolocationReturn {
  const isSupported = 'geolocation' in navigator
  const [position, setPosition] = useState<GeoPosition | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [errorCode, setErrorCode] = useState<GeoErrorCode | null>(null)
  const [attempt, setAttempt] = useState(0)

  const retry = useCallback(() => {
    setError(null)
    setErrorCode(null)
    setAttempt((value) => value + 1)
  }, [])

  useEffect(() => {
    if (!isSupported) {
      setError('Геолокация не поддерживается этим устройством.')
      setErrorCode('unsupported')
      return
    }

    const success = (pos: GeolocationPosition) => {
      setPosition({
        latitude: pos.coords.latitude,
        longitude: pos.coords.longitude,
        accuracy: pos.coords.accuracy,
        timestamp: pos.timestamp,
      })
      setError(null)
      setErrorCode(null)
    }

    const failure = (err: GeolocationPositionError) => {
      switch (err.code) {
        case err.PERMISSION_DENIED:
          setError('Доступ к геолокации запрещён. Разрешите его в настройках Telegram или браузера.')
          setErrorCode('permission-denied')
          break
        case err.POSITION_UNAVAILABLE:
          setError('Не удалось определить местоположение. Проверьте GPS и подключение к сети.')
          setErrorCode('unavailable')
          break
        case err.TIMEOUT:
          setError('Геолокация не ответила вовремя. Попробуйте ещё раз.')
          setErrorCode('timeout')
          break
        default:
          setError('Неизвестная ошибка геолокации.')
          setErrorCode('unknown')
      }
    }

    const watchId = navigator.geolocation.watchPosition(success, failure, {
      enableHighAccuracy: env.geolocation.enableHighAccuracy,
      timeout: env.geolocation.timeoutMs,
      maximumAge: env.geolocation.maximumAgeMs,
    })

    return () => navigator.geolocation.clearWatch(watchId)
  }, [isSupported, attempt])

  return { position, error, errorCode, isSupported, retry }
}
