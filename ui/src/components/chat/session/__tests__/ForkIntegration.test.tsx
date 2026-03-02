/**
 * ForkIntegration Tests
 *
 * Tests for the ForkIntegration component and related hooks/utilities.
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import ForkIntegration, { useForkActions, SessionListForkHandler } from '../ForkIntegration'
import { ForkProvider } from '../ForkContext'
import type { Session, Message, ProviderInfo } from '../../../../types/chat'

// Mock the useFork hook
const mockUseForkReturn = {
  isForkDialogOpen: false,
  sessionToFork: undefined,
  forkPointMessage: undefined,
  isForking: false,
  forkError: undefined,
  lastForkResult: undefined,
  forkInfo: undefined,
  forkInfoLoading: false,
  sessionForks: [],
  sessionForksLoading: false,
  openForkDialog: vi.fn(),
  closeForkDialog: vi.fn(),
  forkSession: vi.fn(),
  getForkInfo: vi.fn().mockResolvedValue(undefined),
  listSessionForks: vi.fn().mockResolvedValue([]),
  clearForkError: vi.fn(),
}

vi.mock('../../../../hooks/useFork', () => ({
  useFork: vi.fn(() => mockUseForkReturn),
}))

// Test fixtures
const mockSession: Session = {
  id: 'session-123',
  title: 'Test Session',
  projectPath: '/test/path',
  provider: 'anthropic',
  model: 'claude-3-sonnet',
  status: 'active',
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
}

const mockMessage: Message = {
  id: 'msg-123',
  sessionId: 'session-123',
  role: 'assistant',
  content: 'Test message',
  tokensInput: 100,
  tokensOutput: 50,
  costUsd: 0.001,
  createdAt: '2024-01-01T00:00:00Z',
}

const mockMessages: Message[] = [
  {
    id: 'msg-1',
    sessionId: 'session-123',
    role: 'user',
    content: 'Hello',
    tokensInput: 10,
    tokensOutput: 0,
    costUsd: 0,
    createdAt: '2024-01-01T00:00:00Z',
  },
  mockMessage,
]

const mockProviders: ProviderInfo[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    isAvailable: true,
    models: [
      {
        id: 'claude-3-sonnet',
        name: 'Claude 3 Sonnet',
        provider: 'anthropic',
        contextWindow: 200000,
        maxOutputTokens: 4096,
        pricePerMInputTokens: 3,
        pricePerMOutputTokens: 15,
        supportsTools: true,
        supportsStreaming: true,
      },
    ],
  },
]

// Mock ChatMain component
vi.mock('../../layout/ChatMain', () => ({
  default: function MockChatMain(props: Record<string, unknown>) {
    return (
      <div data-testid="chat-main">
        <span data-testid="has-fork-handler">
          {(typeof props.onForkAtMessage === 'function').toString()}
        </span>
        <span data-testid="has-session-fork">
          {(typeof props.onForkSession === 'function').toString()}
        </span>
        <button
          data-testid="trigger-fork-at-message"
          onClick={() => {
            if (typeof props.onForkAtMessage === 'function') {
              props.onForkAtMessage(mockMessage)
            }
          }}
        >
          Fork at message
        </button>
        <button
          data-testid="trigger-fork-session"
          onClick={() => {
            if (typeof props.onForkSession === 'function') {
              props.onForkSession()
            }
          }}
        >
          Fork session
        </button>
      </div>
    )
  },
}))

describe('ForkIntegration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset the mock return value
    mockUseForkReturn.openForkDialog.mockClear()
    mockUseForkReturn.closeForkDialog.mockClear()
    mockUseForkReturn.getForkInfo.mockClear()
    mockUseForkReturn.listSessionForks.mockClear()
  })

  describe('Component Rendering', () => {
    it('renders ChatMain with fork handlers', () => {
      render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      expect(screen.getByTestId('chat-main')).toBeInTheDocument()
      expect(screen.getByTestId('has-fork-handler')).toHaveTextContent('true')
      expect(screen.getByTestId('has-session-fork')).toHaveTextContent('true')
    })

    it('renders without session', () => {
      render(
        <ForkIntegration
          messages={[]}
          apiBaseUrl="http://test.com"
        />
      )

      expect(screen.getByTestId('chat-main')).toBeInTheDocument()
    })

    it('passes providers to ForkProvider', () => {
      render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          providers={mockProviders}
          apiBaseUrl="http://test.com"
        />
      )

      expect(screen.getByTestId('chat-main')).toBeInTheDocument()
    })
  })

  describe('Fork Actions', () => {
    it('opens fork dialog when fork at message is triggered', async () => {
      render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      fireEvent.click(screen.getByTestId('trigger-fork-at-message'))

      await waitFor(() => {
        expect(mockUseForkReturn.openForkDialog).toHaveBeenCalledWith(
          mockSession,
          mockMessage
        )
      })
    })

    it('opens fork dialog when fork session is triggered', async () => {
      render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      fireEvent.click(screen.getByTestId('trigger-fork-session'))

      await waitFor(() => {
        expect(mockUseForkReturn.openForkDialog).toHaveBeenCalledWith(
          mockSession,
          mockMessage // Latest message
        )
      })
    })
  })

  describe('Fork Info Fetching', () => {
    it('fetches fork info when session changes', async () => {
      const { rerender } = render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      await waitFor(() => {
        expect(mockUseForkReturn.getForkInfo).toHaveBeenCalledWith('session-123')
      })

      // Change session
      const newSession = { ...mockSession, id: 'session-456' }
      rerender(
        <ForkIntegration
          session={newSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      await waitFor(() => {
        expect(mockUseForkReturn.getForkInfo).toHaveBeenCalledWith('session-456')
      })
    })

    it('fetches session forks when session changes', async () => {
      render(
        <ForkIntegration
          session={mockSession}
          messages={mockMessages}
          apiBaseUrl="http://test.com"
        />
      )

      await waitFor(() => {
        expect(mockUseForkReturn.listSessionForks).toHaveBeenCalledWith('session-123')
      })
    })
  })
})

describe('useForkActions', () => {
  // Test component that uses the hook
  const TestForkActions: React.FC = () => {
    const actions = useForkActions()
    return (
      <div>
        <span data-testid="can-fork">{actions.canFork.toString()}</span>
        <span data-testid="is-forking">{actions.isForking.toString()}</span>
        <button
          data-testid="fork-session"
          onClick={() => {
            actions.forkSession(mockSession)
          }}
        >
          Fork
        </button>
        <button
          data-testid="close-dialog"
          onClick={() => {
            actions.closeForkDialog()
          }}
        >
          Close
        </button>
      </div>
    )
  }

  it('provides fork capabilities', () => {
    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <TestForkActions />
      </ForkProvider>
    )

    expect(screen.getByTestId('can-fork')).toHaveTextContent('true')
    expect(screen.getByTestId('is-forking')).toHaveTextContent('false')
  })

  it('opens fork dialog when forkSession is called', () => {
    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <TestForkActions />
      </ForkProvider>
    )

    fireEvent.click(screen.getByTestId('fork-session'))
    expect(mockUseForkReturn.openForkDialog).toHaveBeenCalledWith(mockSession, undefined)
  })

  it('closes dialog when closeForkDialog is called', () => {
    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <TestForkActions />
      </ForkProvider>
    )

    fireEvent.click(screen.getByTestId('close-dialog'))
    expect(mockUseForkReturn.closeForkDialog).toHaveBeenCalled()
  })
})

describe('SessionListForkHandler', () => {
  const mockSessions = [
    { id: 'session-1', title: 'Session 1', provider: 'anthropic', model: 'claude-3-sonnet' },
    { id: 'session-2', title: 'Session 2', provider: 'openai', model: 'gpt-4' },
  ]

  it('renders children with fork handler', () => {
    const mockOnSelect = vi.fn()

    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <SessionListForkHandler
          sessions={mockSessions}
          onSelectSession={mockOnSelect}
        >
          {({ sessions, onFork, onSelectSession }) => (
            <ul>
              {sessions.map((s) => (
                <li key={s.id}>
                  <button
                    data-testid={`select-${s.id}`}
                    onClick={() => onSelectSession(s.id)}
                  >
                    {s.title}
                  </button>
                  <button
                    data-testid={`fork-${s.id}`}
                    onClick={() => onFork(s.id)}
                  >
                    Fork
                  </button>
                </li>
              ))}
            </ul>
          )}
        </SessionListForkHandler>
      </ForkProvider>
    )

    expect(screen.getByTestId('select-session-1')).toBeInTheDocument()
    expect(screen.getByTestId('fork-session-1')).toBeInTheDocument()
  })

  it('opens fork dialog when onFork is called', () => {
    const mockOnSelect = vi.fn()

    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <SessionListForkHandler
          sessions={mockSessions}
          onSelectSession={mockOnSelect}
        >
          {({ onFork }) => (
            <button
              data-testid="fork-button"
              onClick={() => onFork('session-1')}
            >
              Fork Session 1
            </button>
          )}
        </SessionListForkHandler>
      </ForkProvider>
    )

    fireEvent.click(screen.getByTestId('fork-button'))

    expect(mockUseForkReturn.openForkDialog).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'session-1',
        title: 'Session 1',
        provider: 'anthropic',
        model: 'claude-3-sonnet',
      })
    )
  })

  it('calls onSelectSession when session is selected', () => {
    const mockOnSelect = vi.fn()

    render(
      <ForkProvider apiBaseUrl="http://test.com">
        <SessionListForkHandler
          sessions={mockSessions}
          onSelectSession={mockOnSelect}
        >
          {({ onSelectSession }) => (
            <button
              data-testid="select-button"
              onClick={() => onSelectSession('session-2')}
            >
              Select Session 2
            </button>
          )}
        </SessionListForkHandler>
      </ForkProvider>
    )

    fireEvent.click(screen.getByTestId('select-button'))
    expect(mockOnSelect).toHaveBeenCalledWith('session-2')
  })
})
