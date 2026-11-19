import { useEffect, useState } from 'react'
import { env } from '../config/env'

const NOMINATIM_URL = env.places.nominatimUrl.replace(/\/search\/?$/, '/reverse')
const DEBOUNCE_MS = env.reverseGeocode.debounceMs

function buildParams(lat: number, lon: number): string {
  const url = new URL(NOMINATIM_URL)
  url.searchParams.set('format', 'jsonv2')
  url.searchParams.set('lat', String(lat))
  url.searchParams.set('lon', String(lon))
  url.searchParams.set('zoom', String(env.reverseGeocode.zoom))
  url.searchParams.set('addressdetails', '1')
  return url.toString()
}

/**
 * Возвращает название города по координатам или null, если координат нет.
 */
export function useReverseGeocode(
  latitude: number | null,
  longitude: number | null,
  enabled = true
): string | null {
  const [city, setCity] = useState<string | null>(null)

  useEffect(() => {
    if (!enabled || latitude == null || longitude == null) return

    const controller = new AbortController()

    const timer = window.setTimeout(async () => {
      try {
        const response = await fetch(buildParams(latitude, longitude), {
          signal: controller.signal,
          headers: { 'Accept-Language': env.reverseGeocode.language },
        })

        if (!response.ok) throw new Error(`HTTP ${response.status}`)

        const data = await response.json()
        const address = data.address ?? {}

        if (!controller.signal.aborted) {
          setCity(
            address.city ||
              address.town ||
              address.village ||
              address.municipality ||
              null
          )
        }
      } catch {
        if (!controller.signal.aborted) setCity(null)
      }
    }, DEBOUNCE_MS)

    const requestTimeout = window.setTimeout(() => controller.abort(), env.reverseGeocode.timeoutMs)

    return () => {
      window.clearTimeout(timer)
      window.clearTimeout(requestTimeout)
      controller.abort()
    }
  }, [latitude, longitude, enabled])

  if (!enabled || latitude == null || longitude == null) return null
  return city
}
