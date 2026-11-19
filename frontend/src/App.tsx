import { useEffect, useRef, useState, useCallback } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useGeolocation } from './geolocation/useGeolocation'
import { useReverseGeocode } from './geolocation/useReverseGeocode'
import { fetchTopPlaces } from './places/fetchTopPlaces'
import type { Place } from './places/places'
import { PlaceCard } from './components/PlaceCard'

const MOSCOW_CENTER: L.LatLngTuple = [55.751244, 37.618423]

function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    attraction: '#dc2626',
    artwork: '#db2777',
    museum: '#059669',
    gallery: '#7c3aed',
    theatre: '#d97706',
    arts_centre: '#7c3aed',
    planetarium: '#7c3aed',
    library: '#b45309',
    park: '#16a34a',
    alley: '#16a34a',
    garden: '#22c55e',
    nature_reserve: '#15803d',
    zoo: '#16a34a',
    aquarium: '#0891b2',
    theme_park: '#16a34a',
    viewpoint: '#0891b2',
    castle: '#b91c1c',
    fort: '#b91c1c',
    palace: '#b91c1c',
    monument: '#dc2626',
    memorial: '#dc2626',
    ruins: '#a16207',
    archaeological_site: '#a16207',
    city_gate: '#b91c1c',
    church: '#7c3aed',
    cathedral: '#7c3aed',
    monastery: '#7c3aed',
    mosque: '#7c3aed',
    synagogue: '#7c3aed',
  }
  return colors[category] || '#6b7280'
}

function getCategoryEmoji(category: string): string {
  const emojis: Record<string, string> = {
    attraction: '⭐',
    artwork: '🎨',
    museum: '🏛️',
    gallery: '🖼️',
    theatre: '🎭',
    arts_centre: '🎨',
    planetarium: '🪐',
    library: '📚',
    park: '🌳',
    alley: '🌳',
    garden: '🌿',
    nature_reserve: '🌲',
    zoo: '🦁',
    aquarium: '🐠',
    theme_park: '🎢',
    viewpoint: '👀',
    castle: '🏰',
    fort: '🏰',
    palace: '🏰',
    monument: '🗿',
    memorial: '🕯️',
    ruins: '🏚️',
    archaeological_site: '🏺',
    city_gate: '🏛️',
    church: '⛪',
    cathedral: '⛪',
    monastery: '⛪',
    mosque: '🕌',
    synagogue: '🕍',
  }
  return emojis[category] || '📍'
}

function App() {
  const mapRef = useRef<HTMLDivElement | null>(null)
  const mapInstance = useRef<L.Map | null>(null)
  const userMarkerRef = useRef<L.CircleMarker | null>(null)
  const accuracyCircleRef = useRef<L.Circle | null>(null)
  const placeMarkerRefs = useRef<Map<string, L.Marker>>(new Map())
  const loadTimerRef = useRef<number | null>(null)
  const loadAbortRef = useRef<AbortController | null>(null)
  const [isExpanded, setIsExpanded] = useState(false)
  const [selectedPlace, setSelectedPlace] = useState<Place | null>(null)
  const [places, setPlaces] = useState<Place[]>([])
  const [placesLoading, setPlacesLoading] = useState(false)
  const [placesError, setPlacesError] = useState<string | null>(null)

  const { position, error, isSupported } = useGeolocation()

  // Округляем до ~100 м: не дёргаем Nominatim на каждый тик GPS.
  const geocodeLat = position ? Math.round(position.latitude * 1000) / 1000 : null
  const geocodeLon = position ? Math.round(position.longitude * 1000) / 1000 : null
  const geoCity = useReverseGeocode(geocodeLat, geocodeLon, Boolean(position))

  const displayCity =
    geoCity ||
    (position
      ? `${position.latitude.toFixed(4)}, ${position.longitude.toFixed(4)}`
      : 'Москва')

  const isPanelOpen = Boolean(selectedPlace) || isExpanded

  const handleTogglePanel = () => {
    if (selectedPlace) {
      setSelectedPlace(null)
      return
    }
    setIsExpanded((prev) => !prev)
  }

  /**
   * Загружает до 30 самых значимых объектов в текущих границах карты.
   * Вызовы дебаунсятся, предыдущий запрос отменяется.
   */
  const loadPlaces = useCallback((delay: number) => {
    if (loadTimerRef.current !== null) {
      window.clearTimeout(loadTimerRef.current)
    }

    loadTimerRef.current = window.setTimeout(() => {
      loadTimerRef.current = null

      const map = mapInstance.current
      if (!map) return

      loadAbortRef.current?.abort()
      const controller = new AbortController()
      loadAbortRef.current = controller

      const bounds = map.getBounds()
      setPlacesLoading(true)
      setPlacesError(null)

      fetchTopPlaces(
        {
          south: bounds.getSouth(),
          west: bounds.getWest(),
          north: bounds.getNorth(),
          east: bounds.getEast(),
        },
        controller.signal
      )
        .then((fetched) => {
          if (!controller.signal.aborted) setPlaces(fetched)
        })
        .catch((err: unknown) => {
          if (controller.signal.aborted) return
          console.error('Failed to load places:', err)
          setPlacesError('Не удалось загрузить объекты. Подвиньте карту, чтобы повторить.')
        })
        .finally(() => {
          if (!controller.signal.aborted) setPlacesLoading(false)
        })
    }, delay)
  }, [])

  const zoomIn = useCallback(() => {
    const map = mapInstance.current
    if (map) map.zoomIn()
  }, [])

  const zoomOut = useCallback(() => {
    const map = mapInstance.current
    if (map) map.zoomOut()
  }, [])

  useEffect(() => {
    if (!mapRef.current || mapInstance.current) return

    const markers = placeMarkerRefs.current
    const map = L.map(mapRef.current, {
      center: MOSCOW_CENTER,
      zoom: 13,
      zoomControl: false,
      attributionControl: false,
    })

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png').addTo(map)

    // Пока карту двигают или масштабируют — никаких запросов.
    const cancelPendingLoad = () => {
      if (loadTimerRef.current !== null) {
        window.clearTimeout(loadTimerRef.current)
        loadTimerRef.current = null
      }
      loadAbortRef.current?.abort()
      setPlacesLoading(false)
    }

    // После остановки ждём 500 мс и только затем запрашиваем объекты.
    const handleMoveEnd = () => loadPlaces(500)
    map.on('movestart', cancelPendingLoad)
    map.on('zoomstart', cancelPendingLoad)
    map.on('moveend', handleMoveEnd)

    mapInstance.current = map
    loadPlaces(0)

    return () => {
      map.off('movestart', cancelPendingLoad)
      map.off('zoomstart', cancelPendingLoad)
      map.off('moveend', handleMoveEnd)
      if (loadTimerRef.current !== null) {
        window.clearTimeout(loadTimerRef.current)
        loadTimerRef.current = null
      }
      loadAbortRef.current?.abort()
      markers.forEach((m) => map.removeLayer(m))
      markers.clear()
      map.remove()
      mapInstance.current = null
      userMarkerRef.current = null
      accuracyCircleRef.current = null
    }
  }, [loadPlaces])

  const renderPlaceMarkers = useCallback(() => {
    const map = mapInstance.current
    if (!map) return

    placeMarkerRefs.current.forEach((m) => map.removeLayer(m))
    placeMarkerRefs.current.clear()

    places.forEach((place) => {
      const marker = L.marker([place.latitude, place.longitude], {
        title: place.name,
        alt: place.name,
        riseOnHover: true,
      })
      const color = getCategoryColor(place.category)
      const emoji = getCategoryEmoji(place.category)
      marker.setIcon(
        L.divIcon({
          className: 'place-marker',
          html: `<div style="background:${color};width:30px;height:30px;border-radius:50%;border:3px solid #fff;box-shadow:0 2px 6px rgba(0,0,0,0.3);display:flex;align-items:center;justify-content:center;font-size:14px;line-height:1">${emoji}</div>`,
          iconSize: [30, 30],
          iconAnchor: [15, 15],
        })
      )
      marker.bindTooltip(place.name, {
        direction: 'top',
        offset: L.point(0, -16),
      })
      marker.on('click', () => {
        setSelectedPlace(place)
        // Сохраняем текущий масштаб — только центрируем карту на объекте.
        map.panTo([place.latitude, place.longitude])
      })
      marker.addTo(map)
      placeMarkerRefs.current.set(place.id, marker)
    })
  }, [places])

  useEffect(() => {
    renderPlaceMarkers()
  }, [renderPlaceMarkers])

  const updateUserMarker = useCallback(() => {
    const map = mapInstance.current
    if (!map || !position) return

    const latlng = L.latLng(position.latitude, position.longitude)

    if (!userMarkerRef.current) {
      userMarkerRef.current = L.circleMarker(latlng, {
        radius: 8,
        fillColor: '#2563eb',
        color: '#fff',
        weight: 2,
        opacity: 1,
        fillOpacity: 1,
      }).addTo(map)

      accuracyCircleRef.current = L.circle(latlng, {
        radius: position.accuracy,
        color: '#2563eb',
        weight: 1,
        opacity: 0.3,
        fillOpacity: 0.1,
      }).addTo(map)
    } else {
      userMarkerRef.current.setLatLng(latlng)
      if (accuracyCircleRef.current) {
        accuracyCircleRef.current.setLatLng(latlng)
        accuracyCircleRef.current.setRadius(position.accuracy)
      }
    }
  }, [position])

  useEffect(() => {
    updateUserMarker()
  }, [updateUserMarker])

  return (
    <div className="app">
      <div className="map" ref={mapRef} />

      {selectedPlace && <PlaceCard place={selectedPlace} />}

      <button className="search" aria-label="Поиск">
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="11" cy="11" r="8" />
          <line x1="21" y1="21" x2="16.65" y2="16.65" />
        </svg>
      </button>

      <button
        className="fab toggle-sheet"
        aria-label={isPanelOpen ? 'Свернуть панель' : 'Развернуть панель'}
        onClick={handleTogglePanel}
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {isPanelOpen ? (
            <>
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </>
          ) : (
            <polyline points="16 6 20 6 20 10" />
          )}
        </svg>
      </button>

      <button
        className="fab ai-guide"
        aria-label="Диалог с AI-гидом"
        onClick={() => setIsExpanded(true)}
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3Z" />
          <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
          <line x1="8" y1="14" x2="16" y2="14" />
        </svg>
      </button>

      <button className="zoom-btn zoom-in" onClick={zoomIn} aria-label="Увеличить">
        +
      </button>
      <button className="zoom-btn zoom-out" onClick={zoomOut} aria-label="Уменьшить">
        −
      </button>

      <div
        className={`bottom-sheet ${isExpanded ? 'expanded' : 'collapsed'}`}
        role="button"
        tabIndex={0}
        aria-label={isExpanded ? 'Свернуть панель' : 'Развернуть панель'}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setIsExpanded((p) => !p)
          }
        }}
      >
        <div className="sheet-header" onClick={() => setIsExpanded((p) => !p)}>
          <h1>{selectedPlace ? selectedPlace.name : displayCity}</h1>
        </div>

        <div className="sheet-content">
        </div>
      </div>

      {!isSupported && (
        <div className="geo-error">
          Геолокация не поддерживается браузером
        </div>
      )}

      {error && (
        <div className="geo-error">{error}</div>
      )}

      {placesLoading && (
        <div className="places-loading">Загрузка достопримечательностей…</div>
      )}

      {placesError && !placesLoading && (
        <div className="places-loading places-error">{placesError}</div>
      )}
    </div>
  )
}

export default App
