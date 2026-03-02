/**
 * ChatMain Component Tests
 *
 * Tests for the main chat area with header, messages, and input.
 *
 * @module components/chat/__tests__/ChatMain.test
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import ChatMain from '../layout/ChatMain'
import type { Session, Message, AgentLoopState } from '../../../types/chat'

// Mock child components to simplify testing
vi.mock('../header/ChatHeader', () => ({
  default: ({ session }: { session?: Session }) => (
    <div data-testid="chat-header">{session?.title ?? 'No session'}</div>
  ),
}))

vi.mock('../messages/MessageList', () => ({
  default: ({ messages }: { messages: Message[] }) => (
    <div data-testid="message-list">Messages: {messages.length}</div>
  ),
}))

vi.mock('../input/ChatInput', () => ({
  default: ({ placeholder, disabled }: { placeholder?: string; disabled?: boolean }) => (
    <div data-testid="chat-input" data-disabled={disabled}>
      {placeholder}
    </div>
  ),
}))

describe('ChatMain', () => {
  const mockSession: Session = {
    id: 'session-1',
    projectPath: '/project',
    provider: 'anthropic',
    model: 'claude-3-5-sonnet-20241022',
    title: 'Test Session',
    status: 'active',
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  }

  const mockMessage: Message = {
    id: 'msg-1',
    sessionId: 'session-1',
    role: 'user',
    content: 'Hello',
    tokensInput: 10,
    tokensOutput: 0,
    costUsd: 0.001,
    createdAt: '2024-01-01T00:00:00Z',
  }

  it('renders empty state when no session is selected', () => {
    render(<ChatMain messages={[]} />)

    expect(screen.getByText('No Session Selected')).toBeInTheDocument()
    expect(
      screen.getByText(/Select an existing session from the sidebar/)
    ).toBeInTheDocument()
  })

  it('renders chat header with session info', () => {
    render(<ChatMain session={mockSession} messages={[]} />)

    expect(screen.getByTestId('chat-header')).toHaveTextContent('Test Session')
  })

  it('renders message list with messages', () => {
    render(<ChatMain session={mockSession} messages={[mockMessage]} />)

    expect(screen.getByTestId('message-list')).toHaveTextContent('Messages: 1')
  })

  it('renders loading state when messagesLoading is true and no messages', () => {
    render(<ChatMain session={mockSession} messages={[]} messagesLoading={true} />)

    expect(screen.getByText('Loading messages...')).toBeInTheDocument()
  })

  it('shows message list when messagesLoading is true but has messages', () => {
    render(
      <ChatMain
        session={mockSession}
        messages={[mockMessage]}
        messagesLoading={true}
      />
    )

    expect(screen.getByTestId('message-list')).toBeInTheDocument()
    expect(screen.queryByText('Loading messages...')).not.toBeInTheDocument()
  })

  it('disables input when no session is selected', () => {
    render(<ChatMain messages={[]} />)

    const input = screen.getByTestId('chat-input')
    expect(input).toHaveAttribute('data-disabled', 'true')
    expect(input).toHaveTextContent('Select or create a session to start chatting...')
  })

  it('disables input when loop is running and not paused', () => {
    const loopState: AgentLoopState = {
      iteration: 1,
      totalTokens: 100,
      totalCostUsd: 0.01,
      isRunning: true,
      isPaused: false,
      iterationsSinceProgress: 0,
    }

    render(
      <ChatMain session={mockSession} messages={[]} loopState={loopState} />
    )

    const input = screen.getByTestId('chat-input')
    expect(input).toHaveAttribute('data-disabled', 'true')
    expect(input).toHaveTextContent('Waiting for assistant response...')
  })

  it('enables input when loop is running but paused', () => {
    const loopState: AgentLoopState = {
      iteration: 1,
      totalTokens: 100,
      totalCostUsd: 0.01,
      isRunning: true,
      isPaused: true,
      iterationsSinceProgress: 0,
    }

    render(
      <ChatMain session={mockSession} messages={[]} loopState={loopState} />
    )

    const input = screen.getByTestId('chat-input')
    expect(input).toHaveAttribute('data-disabled', 'false')
  })

  it('shows submitting message when isSubmitting is true', () => {
    render(
      <ChatMain session={mockSession} messages={[]} isSubmitting={true} />
    )

    const input = screen.getByTestId('chat-input')
    expect(input).toHaveTextContent('Sending message...')
  })
})
