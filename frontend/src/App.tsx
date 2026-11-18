import { useEffect, useRef, useState, useCallback } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useGeolocation } from './geolocation/useGeolocation'
import { useReverseGeocode } from './geolocation/useReverseGeocode'
import { fetchNearbyPlaces } from './geolocation/useOverpassPlaces'
import { Place } from './places/places'
import { PlaceCard } from './components/PlaceCard'

const MOSCOW_CENTER = [55.751244, 37.618423]

function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    памятник: '#dc2626',
    архитектура: '#2563eb',
    храм: '#7c3aed',
    музей: '#059669',
    театр: '#d97706',
    cafe: '#d97706',
    restaurant: '#d97706',
    museum: '#059669',
    theatre: '#d97706',
    attraction: '#dc2626',
    gallery: '#7c3aed',
    viewpoint: '#059669',
  }
  return colors[category] || '#6b7280'
}

function App() {
  const mapRef = useRef<HTMLDivElement | null>(null)
  const mapInstance = useRef<L.Map | null>(null)
  const userMarkerRef = useRef<L.CircleMarker | null>(null)
  const accuracyCircleRef = useRef<L.Circle | null>(null)
  const placeMarkerRefs = useRef<Map<string, L.Marker>>(new Map())
  const [isExpanded, setIsExpanded] = useState(true)
  const [isPicking, setIsPicking] = useState(false)
  const [selectedPlace, setSelectedPlace] = useState<Place | null>(null)
  const [places, setPlaces] = useState<Place[]>([])
  const [placesLoading, setPlacesLoading] = useState(false)

  const {
    position,
    error,
    isSupported,
    isTestMode,
    testPosition,
    setTestPosition,
  } = useGeolocation()

  const activePos = isTestMode ? testPosition : position
  const { city: geoCity } = useReverseGeocode(
    activePos?.latitude ?? null,
    activePos?.longitude ?? null,
    !!activePos
  )

  const displayCity = geoCity || (activePos
    ? `${activePos.latitude.toFixed(4)}, ${activePos.longitude.toFixed(4)}`
    : 'Москва')

  const loadPlaces = useCallback(async () => {
    const pos = activePos
    if (!pos) return
    setPlacesLoading(true)
    try {
      const fetched = await fetchNearbyPlaces(pos.latitude, pos.longitude, 1000)
      setPlaces(fetched)
    } catch (err) {
      console.error('Failed to load places:', err)
    } finally {
      setPlacesLoading(false)
    }
  }, [activePos])

  useEffect(() => {
    const timer = setTimeout(() => {
      loadPlaces()
    }, 800)
    return () => clearTimeout(timer)
  }, [loadPlaces])

  useEffect(() => {
    if (!mapRef.current || mapInstance.current) return

    const map = L.map(mapRef.current, {
      center: MOSCOW_CENTER,
      zoom: 13,
      zoomControl: false,
      attributionControl: false,
    })

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png').addTo(map)

    mapInstance.current = map

    return () => {
      placeMarkerRefs.current.forEach((m) => map.removeLayer(m))
      placeMarkerRefs.current.clear()
      map.remove()
      mapInstance.current = null
      userMarkerRef.current = null
      accuracyCircleRef.current = null
    }
  }, [])

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
      marker.setIcon(
        L.divIcon({
          className: 'place-marker',
          html: `<div style="background:${color};width:28px;height:28px;border-radius:50%;border:3px solid #fff;box-shadow:0 2px 6px rgba(0,0,0,0.3);display:flex;align-items:center;justify-content:center;font-size:12px;color:#fff;font-weight:bold">${place.category.charAt(0).toUpperCase()}</div>`,
          iconSize: [28, 28],
          iconAnchor: [14, 14],
        })
      )
      marker.on('click', () => {
        setSelectedPlace(place)
        map.flyTo([place.latitude, place.longitude], 15)
      })
      marker.addTo(map)
      placeMarkerRefs.current.set(place.id, marker)
    })
  }, [mapInstance, places])

  useEffect(() => {
    renderPlaceMarkers()
  }, [renderPlaceMarkers])

  const updateUserMarker = useCallback(() => {
    const map = mapInstance.current
    const pos = isTestMode ? testPosition : position
    if (!map || !pos) return

    const latlng = L.latLng(pos.latitude, pos.longitude)

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
        radius: pos.accuracy,
        color: '#2563eb',
        weight: 1,
        opacity: 0.3,
        fillOpacity: 0.1,
      }).addTo(map)
    } else {
      userMarkerRef.current.setLatLng(latlng)
      userMarkerRef.current.setStyle({
        radius: 8,
        fillColor: '#2563eb',
        color: '#fff',
        weight: 2,
        opacity: 1,
        fillOpacity: 1,
      })
      if (accuracyCircleRef.current) {
        accuracyCircleRef.current.setLatLng(latlng)
        accuracyCircleRef.current.setRadius(pos.accuracy)
      }
    }
  }, [mapInstance, userMarkerRef, accuracyCircleRef, isTestMode, testPosition, position])

  useEffect(() => {
    updateUserMarker()
  }, [updateUserMarker])

  const handleMapClick = useCallback(
    (e: L.LeafletMouseEvent) => {
      if (!isPicking) return
      setTestPosition({
        latitude: e.latlng.lat,
        longitude: e.latlng.lng,
        accuracy: 10,
        timestamp: Date.now(),
      })
      setIsPicking(false)
    },
    [isPicking, setTestPosition]
  )

  useEffect(() => {
    const map = mapInstance.current
    if (!map) return

    if (isPicking) {
      map.on('click', handleMapClick)
      map.getContainer().style.cursor = 'crosshair'
    } else {
      map.off('click', handleMapClick)
      map.getContainer().style.cursor = ''
    }

    return () => {
      map.off('click', handleMapClick)
      map.getContainer().style.cursor = ''
    }
  }, [isPicking, handleMapClick])

  return (
    <div className="app">
      <div className="map" ref={mapRef} />

      {selectedPlace && (
        <PlaceCard place={selectedPlace} onClose={() => setSelectedPlace(null)} />
      )}

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
        aria-label={isExpanded ? 'Свернуть панель' : 'Развернуть панель'}
        onClick={() => setIsExpanded((p) => !p)}
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
          {isExpanded ? (
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
          <h1>{displayCity}</h1>
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
    </div>
  )
}

export default App
