import { env } from '../config/env'

interface ConfigResponse {
  markerScale?: number
}

/**
 * Загружает runtime-настройки с backend.
 *
 * `MARKER_SCALE` читается backend из .env, поэтому размер маркеров можно
 * менять без пересборки frontend — достаточно перезапустить backend.
 */
export async function fetchMarkerScale(): Promise<number> {
  try {
    const response = await fetch(`${env.apiBaseUrl}/api/v1/config`, {
      signal: AbortSignal.timeout(5_000),
    })

    if (!response.ok) return env.map.markerScale

    const data = (await response.json()) as ConfigResponse
    return typeof data.markerScale === 'number' && data.markerScale > 0
      ? data.markerScale
      : env.map.markerScale
  } catch {
    return env.map.markerScale
  }
}
