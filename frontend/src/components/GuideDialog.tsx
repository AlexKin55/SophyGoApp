import { useEffect, useRef } from 'react'

interface GuideDialogProps {
  active: boolean
  busy: boolean
  error: string | null
  geoError: string | null
  transcript: string
  userTranscript: string
  onToggleTalk: () => void
}

export function GuideDialog({
  active,
  busy,
  error,
  geoError,
  transcript,
  userTranscript,
  onToggleTalk,
}: GuideDialogProps) {
  const bottomRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [transcript, userTranscript, error, geoError])

  const hasMessages = Boolean(transcript || userTranscript)

  return (
    <div className="guide-dialog">
      <button
        className={`guide-talk ${active ? 'guide-talk-active' : ''}`}
        type="button"
        onClick={onToggleTalk}
        disabled={busy}
      >
        {active ? 'Помолчим' : 'Поговорим'}
      </button>

      <div className="guide-dialog-messages">
        {!hasMessages && !error && !geoError && (
          <p className="guide-dialog-empty">
            Выберите объект на карте — Гид сам начнёт рассказ. Или нажмите «Поговорим».
          </p>
        )}

        {userTranscript && (
          <div className="guide-message guide-message-user">
            <span className="guide-message-author">Вы</span>
            <p>{userTranscript}</p>
          </div>
        )}

        {transcript && (
          <div className="guide-message guide-message-guide">
            <span className="guide-message-author">Гид</span>
            <p>{transcript}</p>
          </div>
        )}

        {error && <ErrorBubble text={error} />}
        {geoError && <ErrorBubble text={geoError} />}

        <div ref={bottomRef} />
      </div>
    </div>
  )
}

function ErrorBubble({ text }: { text: string }) {
  return (
    <div className="guide-message guide-message-error" role="alert">
      <span className="guide-message-error-icon" aria-hidden="true">
        !
      </span>
      <p>{text}</p>
    </div>
  )
}
