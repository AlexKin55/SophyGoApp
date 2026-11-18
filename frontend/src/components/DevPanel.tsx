import { useState } from 'react'
import { GeoPosition } from '../geolocation/useGeolocation'

interface DevPanelProps {
  isOpen: boolean
  isTestMode: boolean
  testPosition: GeoPosition | null
  onClose: () => void
  onToggleTestMode: () => void
  onSetTestPosition: (pos: GeoPosition) => void
  onPickFromMap: () => void
  isPicking: boolean
}

export function DevPanel({
  isOpen,
  isTestMode,
  testPosition,
  onClose,
  onToggleTestMode,
  onSetTestPosition,
  onPickFromMap,
  isPicking,
}: DevPanelProps) {
  const [lat, setLat] = useState(testPosition?.latitude.toFixed(6) || '')
  const [lng, setLng] = useState(testPosition?.longitude.toFixed(6) || '')
  const [acc, setAcc] = useState(testPosition?.accuracy.toFixed(0) || '10')

  if (!isOpen) return null

  const handleApply = () => {
    const latitude = parseFloat(lat)
    const longitude = parseFloat(lng)
    const accuracy = parseFloat(acc) || 10
    if (!isNaN(latitude) && !isNaN(longitude)) {
      onSetTestPosition({ latitude, longitude, accuracy, timestamp: Date.now() })
    }
  }

  return (
    <div className="dev-panel">
      <div className="dev-panel-header" onClick={onClose}>
        <span className="dev-title">🔧 Режим разработчика</span>
        <button className="dev-close" aria-label="Закрыть панель разработчика">✕</button>
      </div>

      <div className="dev-section">
        <label className="dev-label">Тестовая геолокация</label>
        <button
          className={`dev-toggle ${isTestMode ? 'active' : ''}`}
          onClick={onToggleTestMode}
          aria-pressed={isTestMode}
        >
          {isTestMode ? 'Включено' : 'Выключено'}
        </button>
      </div>

      {isTestMode && (
        <>
          <div className="dev-section">
            <button
              className={`dev-btn ${isPicking ? 'active' : ''}`}
              onClick={onPickFromMap}
              title="Кликните на карте, чтобы выбрать точку"
            >
              {isPicking ? 'Выбор на карте…' : 'Выбрать точку на карте'}
            </button>
          </div>

          <div className="dev-section">
            <label className="dev-label">Широта</label>
            <input
              className="dev-input"
              type="text"
              value={lat}
              onChange={(e) => setLat(e.target.value)}
              placeholder="55.751244"
            />
          </div>

          <div className="dev-section">
            <label className="dev-label">Долгота</label>
            <input
              className="dev-input"
              type="text"
              value={lng}
              onChange={(e) => setLng(e.target.value)}
              placeholder="37.618423"
            />
          </div>

          <div className="dev-section">
            <label className="dev-label">Точность (м)</label>
            <input
              className="dev-input"
              type="text"
              value={acc}
              onChange={(e) => setAcc(e.target.value)}
              placeholder="10"
            />
          </div>

          <button className="dev-btn" onClick={handleApply}>
            Применить координаты
          </button>

          {testPosition && (
            <div className="dev-hint">
              Текущая тестовая позиция: {testPosition.latitude.toFixed(6)},{' '}
              {testPosition.longitude.toFixed(6)}
            </div>
          )}
        </>
      )}
    </div>
  )
}
