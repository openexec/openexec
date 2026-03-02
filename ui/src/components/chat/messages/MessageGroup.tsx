/**
 * MessageGroup Component
 * Groups consecutive messages from same role.
 * @module components/chat/messages/MessageGroup
 */
import React from 'react'
import type { Message, MessageRole } from '../../../types/chat'

export interface MessageGroupProps {
  messages: Message[]
  role: MessageRole
}

const MessageGroup: React.FC<MessageGroupProps> = ({ messages, role }) => {
  return (
    <div className={`message-group message-group--${role}`} style={styles.container}>
      {messages.map((msg) => (
        <div key={msg.id} className="message-group__item" style={styles.item}>
          {msg.content}
        </div>
      ))}
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  item: {
    fontSize: '14px',
    lineHeight: 1.5,
  },
}

export default MessageGroup
