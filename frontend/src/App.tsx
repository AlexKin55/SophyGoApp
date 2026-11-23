import { useEffect, useRef, useState, useCallback } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { useGeolocation } from './geolocation/useGeolocation'
import { useReverseGeocode } from './geolocation/useReverseGeocode'
import { fetchTopPlaces } from './places/fetchTopPlaces'
import { fetchPlacePhotos } from './places/fetchPlacePhotos'
import { fetchMarkerScale } from './api/config'
import type { Place } from './places/places'
import { GuideDialog } from './components/GuideDialog'
import { PlacePhotoCarousel } from './components/PlacePhotoCarousel'
import { useGuideSession } from './guide/useGuideSession'
import { env } from './config/env'
import { logger } from './config/logger'

const MOSCOW_CENTER: L.LatLngTuple = env.map.center

function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    military: '#dc2626',
    historic_building: '#92400e',
    attraction: '#0ea5e9',
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
    military: '⭐',
    historic_building: '🏠',
    attraction: '🧭',
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
  const placesRef = useRef<Place[]>([])
  const [isExpanded, setIsExpanded] = useState(false)
  const [selectedPlace, setSelectedPlace] = useState<Place | null>(null)
  const [places, setPlaces] = useState<Place[]>([])
  const [placesLoading, setPlacesLoading] = useState(false)
  const [placesError, setPlacesError] = useState<string | null>(null)
  const [markerScale, setMarkerScale] = useState(env.map.markerScale)

  const guide = useGuideSession()
  const handleGuidePlaceOpen = guide.handlePlaceOpen
  const { position, error, errorCode, retry } = useGeolocation()

  const selectedPlaceId = selectedPlace?.id ?? null

  useEffect(() => {
    void fetchMarkerScale().then(setMarkerScale)
  }, [])

  useEffect(() => {
    placesRef.current = places
  }, [places])

  // При выборе объекта догружаем дополнительные фото (до шести).
  // Первое фото из списка уже показано, остальные появятся по мере загрузки.
  useEffect(() => {
    if (!selectedPlaceId) return
    const place = placesRef.current.find((item) => item.id === selectedPlaceId)
    if (!place) return

    const controller = new AbortController()
    void fetchPlacePhotos(place, controller.signal)
      .then((photos) => {
        if (controller.signal.aborted) return
        setSelectedPlace((current) =>
          current && current.id === selectedPlaceId && photos.length > (current.images?.length ?? 0)
            ? { ...current, images: photos }
            : current
        )
      })
      .catch((err: unknown) => {
        logger.warn('не удалось догрузить фото объекта', {
          place: place.name,
          error: String(err),
        })
      })

    return () => controller.abort()
  }, [selectedPlaceId])

  // Округляем до ~100 м: не дёргаем Nominatim на каждый тик GPS.
  const geocodeLat = position ? Math.round(position.latitude * env.geolocation.roundFactor) / env.geolocation.roundFactor : null
  const geocodeLon = position ? Math.round(position.longitude * env.geolocation.roundFactor) / env.geolocation.roundFactor : null
  const geoCity = useReverseGeocode(geocodeLat, geocodeLon, Boolean(position))

  // В панели показываем название места, а не координаты.
  const displayCity = geoCity || (position ? 'Ваше местоположение' : 'Москва')

  const guideActive = guide.listening || guide.status === 'speaking'
  const guideBusy = guide.status === 'connecting'

  const handleToggleTalk = () => {
    if (guideActive) {
      void guide.stop()
      if (guide.listening) {
        void guide.toggleListening()
      }
      return
    }
    void guide.toggleListening()
  }

  const isDrawerOpen = Boolean(selectedPlace) || isExpanded

  const handleToggleDrawer = () => {
    if (isDrawerOpen) {
      setIsExpanded(false)
      setSelectedPlace(null)
      return
    }
    setIsExpanded(true)
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
          logger.error('не удалось загрузить объекты', { error: String(err) })
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
    const container = mapRef.current
    if (!container || mapInstance.current) return

    const markers = placeMarkerRefs.current
    const map = L.map(container, {
      center: MOSCOW_CENTER,
      zoom: env.map.zoom,
      zoomControl: false,
      attributionControl: false,
    })

    L.tileLayer(env.map.tileUrl).addTo(map)

    // Карта занимает треть экрана и меняет размер при повороте устройства.
    const resizeObserver = new ResizeObserver(() => map.invalidateSize())
    resizeObserver.observe(container)

    // Пока карту двигают или масштабируют — никаких запросов.
    const cancelPendingLoad = () => {
      if (loadTimerRef.current !== null) {
        window.clearTimeout(loadTimerRef.current)
        loadTimerRef.current = null
      }
      loadAbortRef.current?.abort()
      setPlacesLoading(false)
    }

    // После остановки карты ждём moveDebounceMs и только затем запрашиваем объекты.
    const handleMoveEnd = () => loadPlaces(env.map.moveDebounceMs)
    map.on('movestart', cancelPendingLoad)
    map.on('zoomstart', cancelPendingLoad)
    map.on('moveend', handleMoveEnd)

    mapInstance.current = map
    loadPlaces(0)

    return () => {
      resizeObserver.disconnect()
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
      const markerSize = Math.round(30 * markerScale)
      const markerBorder = Math.max(1, Math.round((3 * markerScale) / 2))
      const markerFontSize = Math.round(14 * markerScale)

      const markerElement = document.createElement('div')
      markerElement.className = 'place-marker-inner'
      markerElement.style.width = `${markerSize}px`
      markerElement.style.height = `${markerSize}px`
      markerElement.style.boxSizing = 'border-box'
      markerElement.style.borderRadius = '50%'
      markerElement.style.border = `${markerBorder}px solid #fff`
      markerElement.style.boxShadow = '0 2px 6px rgba(0,0,0,0.3)'
      markerElement.style.overflow = 'hidden'

      const showEmojiFallback = () => {
        markerElement.replaceChildren()
        markerElement.style.background = color
        markerElement.style.display = 'flex'
        markerElement.style.alignItems = 'center'
        markerElement.style.justifyContent = 'center'
        markerElement.style.fontSize = `${markerFontSize}px`
        markerElement.style.lineHeight = '1'
        markerElement.textContent = emoji
      }

      const photo = place.images?.[0]
      if (photo) {
        markerElement.style.background = '#fff'
        const image = document.createElement('img')
        image.src = photo
        image.alt = place.name
        image.decoding = 'async'
        image.style.width = '100%'
        image.style.height = '100%'
        image.style.objectFit = 'cover'
        image.style.display = 'block'
        image.onerror = showEmojiFallback
        markerElement.appendChild(image)
      } else {
        showEmojiFallback()
      }

      marker.setIcon(
        L.divIcon({
          className: 'place-marker',
          html: markerElement,
          iconSize: [markerSize, markerSize],
          iconAnchor: [markerSize / 2, markerSize / 2],
        })
      )
      marker.bindTooltip(place.name, {
        direction: 'top',
        offset: L.point(0, -16),
      })
      marker.on('click', () => {
        setSelectedPlace(place)
        // Сообщаем backend, что пользователь открыл исторический объект.
        // Сервис передаёт объект в голосовую сессию Гида.
        void handleGuidePlaceOpen(place)
        // Сохраняем текущий масштаб — только центрируем карту на объекте.
        map.panTo([place.latitude, place.longitude])
      })
      marker.addTo(map)
      placeMarkerRefs.current.set(place.id, marker)
    })
  }, [places, handleGuidePlaceOpen, markerScale])

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
      <div className="map-pane">
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

      <button className="zoom-btn zoom-in" onClick={zoomIn} aria-label="Увеличить">
        +
      </button>
      <button className="zoom-btn zoom-out" onClick={zoomOut} aria-label="Уменьшить">
        −
      </button>

      {(error || guide.micError) && (
        <div className="notice-stack">
          {error && (
            <div className="geo-notice" role="status">
              <div className="geo-notice-text">
                <span>{error}</span>
                {errorCode === 'permission-denied' && (
                  <span className="geo-notice-hint">
                    iPhone: Настройки → Telegram → Геопозиция. Android: Настройки → Приложения → Telegram → Разрешения.
                  </span>
                )}
              </div>
              <button type="button" className="geo-notice-retry" onClick={retry}>
                Повторить
              </button>
            </div>
          )}

          {guide.micError && (
            <div className="geo-notice" role="status">
              <div className="geo-notice-text">
                <span>{guide.micError}</span>
                {guide.micErrorCode === 'permission-denied' && (
                  <span className="geo-notice-hint">
                    iPhone: Настройки → Telegram → Микрофон. Android: Настройки → Приложения → Telegram → Разрешения → Микрофон.
                  </span>
                )}
              </div>
              <button type="button" className="geo-notice-retry" onClick={guide.retryMicrophone}>
                Повторить
              </button>
            </div>
          )}
        </div>
      )}

      {placesLoading && (
        <div className="places-loading">Загрузка достопримечательностей…</div>
      )}

      {placesError && !placesLoading && (
        <div className="places-loading places-error">{placesError}</div>
      )}
      </div>

      <div className={`drawer ${isDrawerOpen ? 'open' : 'closed'}`}>
        <div
          className={`drawer-header ${selectedPlace ? 'drawer-header-compact' : ''}`}
          role="button"
          tabIndex={0}
          aria-label={isDrawerOpen ? 'Свернуть панель' : 'Развернуть панель'}
          onClick={handleToggleDrawer}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              handleToggleDrawer()
            }
          }}
        >
          <span className="drawer-handle" aria-hidden="true" />
          {/* Название объекта показываем на фото, в заголовке — город/локация. */}
          {!selectedPlace && <h1>{displayCity}</h1>}
        </div>

        <div className="drawer-content">
          <div className="photo-pane">
            {selectedPlace ? (
              <PlacePhotoCarousel
                key={selectedPlace.id}
                name={selectedPlace.name}
                images={selectedPlace.images ?? []}
              />
            ) : (
              <div className="photo-pane-placeholder">
                <span aria-hidden="true">📍</span>
                <p>{displayCity}</p>
              </div>
            )}
          </div>

          <div className="dialog-pane">
            <GuideDialog
              active={guideActive}
              busy={guideBusy}
              error={guide.error}
              transcript={guide.transcript}
              userTranscript={guide.userTranscript}
              onToggleTalk={handleToggleTalk}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

export default App
