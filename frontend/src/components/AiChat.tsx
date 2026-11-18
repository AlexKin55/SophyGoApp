import { useState } from 'react'

interface AiChatProps {
  isOpen: boolean
  onClose: () => void
}

export function AiChat({ isOpen, onClose }: AiChatProps) {
  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<{ text: string; from: 'ai' | 'user' }[]>([
    { text: 'Привет! Я твой AI-гид. Выбери достопримечательность на карте, и я расскажу о ней.', from: 'ai' },
  ])

  const sendMessage = () => {
    if (!input.trim()) return
    setMessages([...messages, { text: input, from: 'user' }])
    setInput('')
    setMessages([...messages, { text: 'Скоро подключим реального AI-гида.', from: 'ai' }])
  }

  if (!isOpen) return null

  return (
    <div className="ai-chat">
      <div className="ai-chat-header" onClick={onClose}>
        <span className="ai-chat-title">AI-гид</span>
        <button className="ai-chat-close" aria-label="Закрыть чат">✕</button>
      </div>
      <div className="ai-chat-messages">
        {messages.map((m, i) => (
          <div
            key={i}
            className={`ai-message ${m.from === 'ai' ? 'ai-message-ai' : 'ai-message-user'}`}
          >
            {m.text}
          </div>
        ))}
      </div>
      <div className="ai-chat-input">
        <input
          className="ai-chat-input-field"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Задайте вопрос гиду…"
          onKeyDown={(e) => {
            if (e.key === 'Enter') sendMessage()
          }}
        />
        <button className="ai-chat-send" onClick={sendMessage} aria-label="Отправить">
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <line x1="22" y1="2" x2="11" y2="13" />
            <polygon points="22 2 15 22 11 13 2 9 22 2" />
          </svg>
        </button>
      </div>
    </div>
  )
}
