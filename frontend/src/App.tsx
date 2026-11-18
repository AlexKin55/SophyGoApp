import { useEffect, useRef, useState, useCallback } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useGeolocation } from './geolocation/useGeolocation'
import { useReverseGeocode } from './geolocation/useReverseGeocode'

const MOSCOW_CENTER = [55.751244, 37.618423]

function App() {
  const mapRef = useRef<HTMLDivElement | null>(null)
  const mapInstance = useRef<L.Map | null>(null)
  const userMarkerRef = useRef<L.CircleMarker | null>(null)
  const accuracyCircleRef = useRef<L.Circle | null>(null)
  const [isExpanded, setIsExpanded] = useState(true)
  const [isPicking, setIsPicking] = useState(false)

  const {
    position,
    error,
    isSupported,
    isTestMode,
    testPosition,
    setTestPosition,
  } = useGeolocation()

  const activePos = isTestMode ? testPosition : position
  const { city: geoCity, isLoading: geoLoading } = useReverseGeocode(
    activePos?.latitude ?? null,
    activePos?.longitude ?? null,
    !!activePos
  )

  const displayCity = geoCity || (activePos
    ? `${activePos.latitude.toFixed(4)}, ${activePos.longitude.toFixed(4)}`
    : 'Москва')

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
      map.remove()
      mapInstance.current = null
      userMarkerRef.current = null
      accuracyCircleRef.current = null
    }
  }, [])

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
        onClick={() => setAiChatOpen(true)}
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
          {isExpanded && (
            <>
              <div className="tabs">
                <button className="tab active">Места</button>
                <button className="tab">Районы</button>
                <button className="tab">Маршруты</button>
              </div>

              <div className="categories">
                <button className="chip active">Еда</button>
                <button className="chip">Напитки</button>
                <button className="chip">Кафе</button>
                <button className="chip">Магазины</button>
                <button className="chip">Культура</button>
              </div>
            </>
          )}
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
    </div>
  )
}

export default App
