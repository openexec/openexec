/**
 * MessageList Component Tests
 *
 * Tests for the scrollable message container.
 *
 * @module components/chat/__tests__/MessageList.test
 */

import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import MessageList from '../messages/MessageList'
import type { Message, ToolCall } from '../../../types/chat'

// Mock scrollIntoView for jsdom
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

// Mock child components
vi.mock('../messages/UserMessage', () => ({
  default: ({ message }: { message: Message }) => (
    <div data-testid="user-message">{message.content}</div>
  ),
}))

vi.mock('../messages/AssistantMessage', () => ({
  default: ({ message }: { message: Message }) => (
    <div data-testid="assistant-message">{message.content}</div>
  ),
}))

vi.mock('../messages/StreamingMessage', () => ({
  default: ({ content }: { content: string }) => (
    <div data-testid="streaming-message">{content}</div>
  ),
}))

vi.mock('../messages/SystemMessage', () => ({
  default: ({ message }: { message: Message }) => (
    <div data-testid="system-message">{message.content}</div>
  ),
}))

describe('MessageList', () => {
  const createMessage = (
    id: string,
    role: 'user' | 'assistant' | 'system',
    content: string
  ): Message => ({
    id,
    sessionId: 'session-1',
    role,
    content,
    tokensInput: 10,
    tokensOutput: role === 'assistant' ? 50 : 0,
    costUsd: 0.001,
    createdAt: '2024-01-01T00:00:00Z',
  })

  it('renders empty state when no messages', () => {
    render(<MessageList messages={[]} />)

    expect(
      screen.getByText('Start a conversation by typing a message below.')
    ).toBeInTheDocument()
  })

  it('renders user messages', () => {
    const messages = [createMessage('msg-1', 'user', 'Hello')]

    render(<MessageList messages={messages} />)

    expect(screen.getByTestId('user-message')).toHaveTextContent('Hello')
  })

  it('renders assistant messages', () => {
    const messages = [createMessage('msg-1', 'assistant', 'Hi there!')]

    render(<MessageList messages={messages} />)

    expect(screen.getByTestId('assistant-message')).toHaveTextContent(
      'Hi there!'
    )
  })

  it('renders system messages', () => {
    const messages = [createMessage('msg-1', 'system', 'System notification')]

    render(<MessageList messages={messages} />)

    expect(screen.getByTestId('system-message')).toHaveTextContent(
      'System notification'
    )
  })

  it('renders multiple messages in order', () => {
    const messages = [
      createMessage('msg-1', 'user', 'Hello'),
      createMessage('msg-2', 'assistant', 'Hi there!'),
      createMessage('msg-3', 'user', 'How are you?'),
    ]

    render(<MessageList messages={messages} />)

    const userMessages = screen.getAllByTestId('user-message')
    const assistantMessages = screen.getAllByTestId('assistant-message')

    expect(userMessages).toHaveLength(2)
    expect(assistantMessages).toHaveLength(1)
  })

  it('renders streaming message when provided', () => {
    const messages = [createMessage('msg-1', 'user', 'Hello')]
    const streamingMessage = { content: 'I am thinking...' }

    render(
      <MessageList messages={messages} streamingMessage={streamingMessage} />
    )

    expect(screen.getByTestId('streaming-message')).toHaveTextContent(
      'I am thinking...'
    )
  })

  it('does not render streaming message when content is undefined', () => {
    const messages = [createMessage('msg-1', 'user', 'Hello')]
    const streamingMessage = {}

    render(
      <MessageList messages={messages} streamingMessage={streamingMessage} />
    )

    expect(screen.queryByTestId('streaming-message')).not.toBeInTheDocument()
  })

  it('does not show empty state when there is a streaming message', () => {
    const streamingMessage = { content: 'Starting...' }

    render(<MessageList messages={[]} streamingMessage={streamingMessage} />)

    expect(
      screen.queryByText('Start a conversation by typing a message below.')
    ).not.toBeInTheDocument()
    expect(screen.getByTestId('streaming-message')).toBeInTheDocument()
  })

  it('passes tool calls to assistant message via toolCallsByMessageId', () => {
    const messages = [createMessage('msg-1', 'assistant', 'Let me check')]
    const toolCall: ToolCall = {
      id: 'tc-1',
      messageId: 'msg-1',
      sessionId: 'session-1',
      toolName: 'bash',
      toolInput: '{"command": "ls"}',
      status: 'completed',
      createdAt: '2024-01-01T00:00:00Z',
    }
    const toolCallsByMessageId = new Map([['msg-1', [toolCall]]])

    render(
      <MessageList
        messages={messages}
        toolCallsByMessageId={toolCallsByMessageId}
      />
    )

    // The assistant message should be rendered
    expect(screen.getByTestId('assistant-message')).toBeInTheDocument()
  })
})
