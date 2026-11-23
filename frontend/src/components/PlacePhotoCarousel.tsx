import { useCallback, useEffect, useRef, useState } from 'react'
import { photoProxyUrl } from '../places/photoProxy'

interface PlacePhotoCarouselProps {
  name: string
  images: string[]
}

/**
 * Фото выбранного объекта: один слайд на экране, остальные доступны
 * свайпом влево-вправо (scroll-snap). На десктопе добавляются стрелки.
 */
export function PlacePhotoCarousel({ name, images }: PlacePhotoCarouselProps) {
  const [failed, setFailed] = useState<string[]>([])
  const [proxied, setProxied] = useState<Record<string, string>>({})
  const [index, setIndex] = useState(0)
  const trackRef = useRef<HTMLDivElement | null>(null)

  const photos = images.filter((src) => !failed.includes(src))

  useEffect(() => {
    const track = trackRef.current
    if (track) track.scrollLeft = 0
    setIndex(0)
  }, [name])

  useEffect(() => {
    if (index > photos.length - 1) {
      setIndex(Math.max(0, photos.length - 1))
    }
  }, [index, photos.length])

  const handleScroll = useCallback(() => {
    const track = trackRef.current
    if (!track) return
    const width = track.clientWidth
    if (!width) return
    setIndex(Math.round(track.scrollLeft / width))
  }, [])

  const goTo = (next: number) => {
    const track = trackRef.current
    if (!track) return
    track.scrollTo({ left: next * track.clientWidth, behavior: 'smooth' })
  }

  const handleError = (src: string) => {
    // В MAX первая ошибка — пробуем backend-прокси, вторая — убираем фото.
    // В Telegram прокси не используется, поведение прежнее.
    const proxy = proxied[src] ? null : photoProxyUrl(src)
    if (proxy) {
      setProxied((prev) => ({ ...prev, [src]: proxy }))
      return
    }
    setFailed((prev) => (prev.includes(src) ? prev : [...prev, src]))
  }

  if (!photos.length) {
    return (
      <div className="photo-pane-placeholder">
        <span aria-hidden="true">📍</span>
        <p>{name}</p>
      </div>
    )
  }

  return (
    <div className="photo-carousel">
      <div className="photo-track" ref={trackRef} onScroll={handleScroll}>
        {photos.map((src) => (
          <div className="photo-slide" key={src}>
            <img
              src={proxied[src] ?? src}
              alt={name}
              decoding="async"
              onError={() => handleError(src)}
            />
          </div>
        ))}
      </div>

      <div className="photo-pane-title">{name}</div>

      {photos.length > 1 && (
        <>
          <div className="photo-counter">
            {index + 1} / {photos.length}
          </div>
          <button
            type="button"
            className="photo-arrow photo-arrow-prev"
            aria-label="Предыдущее фото"
            disabled={index === 0}
            onClick={() => goTo(Math.max(0, index - 1))}
          >
            ‹
          </button>
          <button
            type="button"
            className="photo-arrow photo-arrow-next"
            aria-label="Следующее фото"
            disabled={index === photos.length - 1}
            onClick={() => goTo(Math.min(photos.length - 1, index + 1))}
          >
            ›
          </button>
          <div className="photo-dots" aria-hidden="true">
            {photos.map((src, i) => (
              <span key={src} className={`photo-dot ${i === index ? 'active' : ''}`} />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
