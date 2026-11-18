import { useState, useEffect, useRef, useCallback } from 'react'

export interface ReverseGeocodeResult {
  city: string | null
  isLoading: boolean
  error: string | null
}

const NOMINATIM_URL = 'https://nominatim.openstreetmap.org/reverse'

function buildParams(lat: number, lon: number) {
  const url = new URL(NOMINATIM_URL)
  url.searchParams.set('format', 'jsonv2')
  url.searchParams.set('lat', String(lat))
  url.searchParams.set('lon', String(lon))
  url.searchParams.set('zoom', '10')
  url.searchParams.set('addressdetails', '1')
  return url.toString()
}

export function useReverseGeocode(
  latitude: number | null,
  longitude: number | null,
  enabled: boolean = true
): ReverseGeocodeResult {
  const [result, setResult] = useState<ReverseGeocodeResult>({
    city: null,
    isLoading: false,
    error: null,
  })
  const abortRef = useRef<AbortController | null>(null)

  const fetchCity = useCallback(
    async (lat: number, lon: number, signal: AbortSignal) => {
      setResult((prev) => ({ ...prev, isLoading: true, error: null }))
      try {
        const response = await fetch(buildParams(lat, lon), {
          signal,
          headers: {
            'Accept-Language': 'ru',
            'User-Agent': 'SophyGo/1.0 (development)',
          },
        })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const data = await response.json()
        const address = data.address || {}
        const city =
          address.city ||
          address.town ||
          address.village ||
          address.municipality ||
          null
        setResult({ city, isLoading: false, error: null })
      } catch (err) {
        if (signal.aborted) return
        setResult({
          city: null,
          isLoading: false,
          error: err instanceof Error ? err.message : 'Ошибка определения города',
        })
      }
    },
    []
  )

  useEffect(() => {
    if (!enabled || latitude == null || longitude == null) {
      setResult({ city: null, isLoading: false, error: null })
      return
    }

    if (abortRef.current) {
      abortRef.current.abort()
    }
    const controller = new AbortController()
    abortRef.current = controller

    const timer = setTimeout(() => {
      fetchCity(latitude, longitude, controller.signal)
    }, 400)

    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [latitude, longitude, enabled, fetchCity])

  return result
}
