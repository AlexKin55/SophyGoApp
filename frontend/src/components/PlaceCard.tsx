import { Place } from '../places/places'

interface PlaceCardProps {
  place: Place | null
  onClose: () => void
}

export function PlaceCard({ place, onClose }: PlaceCardProps) {
  if (!place) return null

  return (
    <div className="place-card">
      <button className="place-card-close" onClick={onClose} aria-label="Закрыть карточку">
        ✕
      </button>

      <div className="place-card-image">
        {place.images.length > 0 ? (
          <img src={place.images[0]} alt={place.name} />
        ) : (
          <div className="place-card-placeholder">Нет изображения</div>
        )}
      </div>

      <div className="place-card-content">
        <div className="place-card-category">{place.category}</div>
        <h2 className="place-card-name">{place.name}</h2>
        <p className="place-card-description">{place.description}</p>
      </div>

      <button className="place-card-listen" aria-label="Послушать рассказ">
        Послушать рассказ
      </button>
    </div>
  )
}
