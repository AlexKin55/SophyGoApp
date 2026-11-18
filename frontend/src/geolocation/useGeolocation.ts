import { useState, useEffect, useRef, useCallback } from 'react'

export interface GeoPosition {
  latitude: number
  longitude: number
  accuracy: number
  timestamp: number
}

export interface UseGeolocationReturn {
  position: GeoPosition | null
  error: string | null
  isLoading: boolean
  isSupported: boolean
  isTestMode: boolean
  testPosition: GeoPosition | null
  setTestPosition: (pos: GeoPosition) => void
  toggleTestMode: () => void
  setTestMode: (enabled: boolean) => void
  startWatching: () => void
  stopWatching: () => void
}

const DEFAULT_TEST_POSITION: GeoPosition = {
  latitude: 55.751244,
  longitude: 37.618423,
  accuracy: 10,
  timestamp: Date.now(),
}

export function useGeolocation(): UseGeolocationReturn {
  const [state, setState] = useState<{
    position: GeoPosition | null
    error: string | null
    isLoading: boolean
    isSupported: boolean
    isTestMode: boolean
    testPosition: GeoPosition | null
  }>({
    position: null,
    error: null,
    isLoading: false,
    isSupported: 'geolocation' in navigator,
    isTestMode: false,
    testPosition: { ...DEFAULT_TEST_POSITION },
  })

  const watchId = useRef<number | null>(null)

  const activePosition = useCallback((): GeoPosition | null => {
    return state.isTestMode ? state.testPosition : state.position
  }, [state.isTestMode, state.testPosition, state.position])

  useEffect(() => {
    if (!state.isSupported || state.isTestMode) return

    setState((prev) => ({ ...prev, isLoading: true, error: null }))

    const success = (pos: GeolocationPosition) => {
      const p: GeoPosition = {
        latitude: pos.coords.latitude,
        longitude: pos.coords.longitude,
        accuracy: pos.coords.accuracy,
        timestamp: pos.timestamp,
      }
      setState((prev) => ({ ...prev, position: p, isLoading: false, error: null }))
    }

    const error = (err: GeolocationPositionError) => {
      let msg = 'Неизвестная ошибка геолокации'
      switch (err.code) {
        case 1:
          msg = 'Доступ к геолокации запрещён пользователем'
          break
        case 2:
          msg = 'Позицию невозможно определить'
          break
        case 3:
          msg = 'Время ожидания геолокации истекло'
          break
      }
      setState((prev) => ({ ...prev, error: msg, isLoading: false }))
    }

    watchId.current = navigator.geolocation.watchPosition(success, error, {
      enableHighAccuracy: true,
      timeout: 10000,
      maximumAge: 5000,
    })

    return () => {
      if (watchId.current !== null) {
        navigator.geolocation.clearWatch(watchId.current)
        watchId.current = null
      }
    }
  }, [state.isSupported, state.isTestMode])

  const setTestPosition = useCallback((pos: GeoPosition) => {
    setState((prev) => ({ ...prev, testPosition: pos, error: null }))
  }, [])

  const toggleTestMode = useCallback(() => {
    setState((prev) => ({ ...prev, isTestMode: !prev.isTestMode, error: null }))
  }, [])

  const setTestMode = useCallback((enabled: boolean) => {
    setState((prev) => ({ ...prev, isTestMode: enabled, error: null }))
  }, [])

  const startWatching = useCallback(() => {
    if (state.isTestMode || !state.isSupported) return
    if (watchId.current !== null) return
    setState((prev) => ({ ...prev, isLoading: true, error: null }))
  }, [state.isTestMode, state.isSupported])

  const stopWatching = useCallback(() => {
    if (watchId.current !== null) {
      navigator.geolocation.clearWatch(watchId.current)
      watchId.current = null
    }
  }, [])

  return {
    position: activePosition(),
    error: state.error,
    isLoading: state.isLoading,
    isSupported: state.isSupported,
    isTestMode: state.isTestMode,
    testPosition: state.testPosition,
    setTestPosition,
    toggleTestMode,
    setTestMode,
    startWatching,
    stopWatching,
  }
}
