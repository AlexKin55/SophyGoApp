import { useEffect, useState } from 'react'
import { env } from '../config/env'

export interface GeoPosition {
  latitude: number
  longitude: number
  accuracy: number
  timestamp: number
}

export interface UseGeolocationReturn {
  position: GeoPosition | null
  error: string | null
  isSupported: boolean
}

export function useGeolocation(): UseGeolocationReturn {
  const isSupported = 'geolocation' in navigator
  const [position, setPosition] = useState<GeoPosition | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!isSupported) return

    const success = (pos: GeolocationPosition) => {
      setPosition({
        latitude: pos.coords.latitude,
        longitude: pos.coords.longitude,
        accuracy: pos.coords.accuracy,
        timestamp: pos.timestamp,
      })
      setError(null)
    }

    const failure = (err: GeolocationPositionError) => {
      let message = 'Неизвестная ошибка геолокации'

      switch (err.code) {
        case err.PERMISSION_DENIED:
          message = 'Доступ к геолокации запрещён пользователем'
          break
        case err.POSITION_UNAVAILABLE:
          message = 'Позицию невозможно определить'
          break
        case err.TIMEOUT:
          message = 'Время ожидания геолокации истекло'
          break
      }

      setError(message)
    }

    const watchId = navigator.geolocation.watchPosition(success, failure, {
      enableHighAccuracy: env.geolocation.enableHighAccuracy,
      timeout: env.geolocation.timeoutMs,
      maximumAge: env.geolocation.maximumAgeMs,
    })

    return () => navigator.geolocation.clearWatch(watchId)
  }, [isSupported])

  return { position, error, isSupported }
}
