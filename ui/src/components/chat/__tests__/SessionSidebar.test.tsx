/**
 * SessionSidebar Component Tests
 *
 * Tests for the session management sidebar.
 *
 * @module components/chat/__tests__/SessionSidebar.test
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import SessionSidebar from '../session/SessionSidebar'
import type { SessionListItem } from '../../../types/chat'

// Mock child components
vi.mock('../session/SessionList', () => ({
  default: ({
    sessions,
    selectedId,
    onSelect,
  }: {
    sessions: SessionListItem[]
    selectedId?: string
    onSelect?: (id: string) => void
  }) => (
    <ul data-testid="session-list">
      {sessions.map((session) => (
        <li
          key={session.id}
          data-testid={`session-${session.id}`}
          data-selected={session.id === selectedId}
          onClick={() => onSelect?.(session.id)}
        >
          {session.title}
        </li>
      ))}
    </ul>
  ),
}))

vi.mock('../session/SessionFilters', () => ({
  default: () => <div data-testid="session-filters" />,
}))

vi.mock('../session/NewSessionButton', () => ({
  default: ({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) => (
    <button data-testid="new-session-button" onClick={onClick} disabled={disabled}>
      New Session
    </button>
  ),
}))

describe('SessionSidebar', () => {
  const mockSessions: SessionListItem[] = [
    {
      id: 'session-1',
      title: 'Session One',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet',
      status: 'active',
      messageCount: 5,
      totalCostUsd: 0.05,
      createdAt: '2024-01-01T00:00:00Z',
      updatedAt: '2024-01-01T00:00:00Z',
    },
    {
      id: 'session-2',
      title: 'Session Two',
      provider: 'openai',
      model: 'gpt-4',
      status: 'active',
      messageCount: 10,
      totalCostUsd: 0.10,
      createdAt: '2024-01-02T00:00:00Z',
      updatedAt: '2024-01-02T00:00:00Z',
    },
  ]

  it('renders header with title', () => {
    render(<SessionSidebar sessions={[]} />)

    expect(screen.getByText('Sessions')).toBeInTheDocument()
  })

  it('renders new session button', () => {
    render(<SessionSidebar sessions={[]} />)

    expect(screen.getByTestId('new-session-button')).toBeInTheDocument()
  })

  it('disables new session button when loading', () => {
    render(<SessionSidebar sessions={[]} loading={true} />)

    expect(screen.getByTestId('new-session-button')).toBeDisabled()
  })

  it('renders session filters', () => {
    render(<SessionSidebar sessions={[]} />)

    expect(screen.getByTestId('session-filters')).toBeInTheDocument()
  })

  it('renders session list with sessions', () => {
    render(<SessionSidebar sessions={mockSessions} />)

    expect(screen.getByTestId('session-list')).toBeInTheDocument()
    expect(screen.getByTestId('session-session-1')).toHaveTextContent('Session One')
    expect(screen.getByTestId('session-session-2')).toHaveTextContent('Session Two')
  })

  it('marks selected session', () => {
    render(
      <SessionSidebar sessions={mockSessions} selectedSessionId="session-1" />
    )

    expect(screen.getByTestId('session-session-1')).toHaveAttribute(
      'data-selected',
      'true'
    )
    expect(screen.getByTestId('session-session-2')).toHaveAttribute(
      'data-selected',
      'false'
    )
  })

  it('calls onSessionSelect when a session is clicked', () => {
    const onSessionSelect = vi.fn()

    render(
      <SessionSidebar sessions={mockSessions} onSessionSelect={onSessionSelect} />
    )

    fireEvent.click(screen.getByTestId('session-session-2'))

    expect(onSessionSelect).toHaveBeenCalledWith('session-2')
  })

  it('shows new session modal when new session button is clicked', () => {
    render(<SessionSidebar sessions={[]} />)

    fireEvent.click(screen.getByTestId('new-session-button'))

    // Modal title and button both have "New Session" text
    const newSessionElements = screen.getAllByText('New Session')
    expect(newSessionElements.length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('Create Session')).toBeInTheDocument()
  })

  it('closes new session modal when cancel is clicked', () => {
    render(<SessionSidebar sessions={[]} />)

    fireEvent.click(screen.getByTestId('new-session-button'))
    expect(screen.getByText('Create Session')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Cancel'))

    // Modal should be closed, only the button's "New Session" text remains
    const newSessionElements = screen.getAllByText('New Session')
    expect(newSessionElements).toHaveLength(1) // Just the button
    expect(screen.queryByText('Create Session')).not.toBeInTheDocument()
  })

  it('calls onNewSession when new session is created', () => {
    const onNewSession = vi.fn()

    render(
      <SessionSidebar
        sessions={[]}
        onNewSession={onNewSession}
        projectPath="/test/project"
        defaultProvider="anthropic"
        defaultModel="claude-3-5-sonnet-20241022"
      />
    )

    // Open modal
    fireEvent.click(screen.getByTestId('new-session-button'))

    // Fill in title
    const titleInput = screen.getByPlaceholderText('Enter session title...')
    fireEvent.change(titleInput, { target: { value: 'My New Session' } })

    // Submit
    fireEvent.click(screen.getByText('Create Session'))

    expect(onNewSession).toHaveBeenCalledWith({
      projectPath: '/test/project',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: 'My New Session',
    })
  })

  it('creates session with empty title when not provided', () => {
    const onNewSession = vi.fn()

    render(
      <SessionSidebar
        sessions={[]}
        onNewSession={onNewSession}
        projectPath="/test/project"
      />
    )

    fireEvent.click(screen.getByTestId('new-session-button'))
    fireEvent.click(screen.getByText('Create Session'))

    expect(onNewSession).toHaveBeenCalledWith({
      projectPath: '/test/project',
      provider: 'anthropic',
      model: 'claude-3-5-sonnet-20241022',
      title: undefined,
    })
  })
})
