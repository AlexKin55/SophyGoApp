import type { Place } from '../places/places'

interface PlaceCardProps {
  place: Place | null
}

export function PlaceCard({ place }: PlaceCardProps) {
  if (!place) return null

  return (
    <div className="place-card">
      <div className="place-card-header">
        <h2 className="place-card-name">{place.name}</h2>
      </div>

      <div className="place-card-media">
        <div className="place-card-guide" aria-label="Фото гида">
          <span className="place-card-guide-icon" aria-hidden="true">
            🧑‍🏫
          </span>
          <span className="place-card-guide-label">Гид</span>
        </div>

        <div className="place-card-image">
          {place.images.length > 0 ? (
            <img src={place.images[0]} alt={place.name} />
          ) : (
            <div className="place-card-placeholder">Нет изображения</div>
          )}
        </div>
      </div>

      <div className="place-card-audio">
        <button
          className="place-card-play"
          type="button"
          aria-label="Воспроизвести аудиогид"
          disabled
        >
          ▶
        </button>
        <div className="place-card-audio-info">
          <span className="place-card-audio-title">Аудиогид от ИИ</span>
          <span className="place-card-audio-note">
            Заглушка — скоро здесь появится рассказ
          </span>
        </div>
      </div>
    </div>
  )
}
