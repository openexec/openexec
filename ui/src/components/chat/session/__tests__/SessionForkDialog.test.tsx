/**
 * Tests for SessionForkDialog Component
 * @module components/chat/session/__tests__/SessionForkDialog
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SessionForkDialog from '../SessionForkDialog'
import type { Session, Message, ProviderInfo } from '../../../../types/chat'
import type { ForkResult } from '../SessionForkDialog'

// Helper to properly resolve async state updates
const flushPromises = () => new Promise(resolve => setTimeout(resolve, 10))

const createMockSession = (overrides: Partial<Session> = {}): Session => ({
  id: 'session-123',
  projectPath: '/test/project',
  provider: 'anthropic',
  model: 'claude-3-5-sonnet',
  title: 'Test Session',
  status: 'active',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  ...overrides,
})

const createMockMessage = (overrides: Partial<Message> = {}): Message => ({
  id: 'msg-123',
  sessionId: 'session-123',
  role: 'assistant',
  content: 'This is a test message content',
  tokensInput: 100,
  tokensOutput: 50,
  costUsd: 0.001,
  createdAt: new Date().toISOString(),
  ...overrides,
})

const createMockForkResult = (overrides: Partial<ForkResult> = {}): ForkResult => ({
  forkedSessionId: 'fork-session-456',
  parentSessionId: 'session-123',
  forkPointMessageId: 'msg-123',
  title: 'Fork of "Test Session"',
  provider: 'anthropic',
  model: 'claude-3-5-sonnet',
  messagesCopied: 5,
  toolCallsCopied: 2,
  summariesCopied: 1,
  forkDepth: 1,
  ancestorChain: ['session-123'],
  ...overrides,
})

const createMockProviders = (): ProviderInfo[] => [
  {
    id: 'anthropic',
    name: 'Anthropic',
    isAvailable: true,
    models: [
      {
        id: 'claude-3-5-sonnet',
        name: 'Claude 3.5 Sonnet',
        provider: 'anthropic',
        contextWindow: 200000,
        maxOutputTokens: 8192,
        pricePerMInputTokens: 3.0,
        pricePerMOutputTokens: 15.0,
        supportsTools: true,
        supportsStreaming: true,
      },
    ],
  },
  {
    id: 'openai',
    name: 'OpenAI',
    isAvailable: true,
    models: [
      {
        id: 'gpt-4o',
        name: 'GPT-4o',
        provider: 'openai',
        contextWindow: 128000,
        maxOutputTokens: 4096,
        pricePerMInputTokens: 5.0,
        pricePerMOutputTokens: 15.0,
        supportsTools: true,
        supportsStreaming: true,
      },
    ],
  },
]

describe('SessionForkDialog', () => {
  const mockOnClose = vi.fn()
  const mockOnFork = vi.fn()
  const mockOnNavigateToSession = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockOnFork.mockResolvedValue(createMockForkResult())
  })

  it('renders nothing when isOpen is false', () => {
    const session = createMockSession()
    const { container } = render(
      <SessionForkDialog
        session={session}
        isOpen={false}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders dialog when isOpen is true', () => {
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Fork Session')).toBeInTheDocument()
  })

  it('displays the parent session information', () => {
    const session = createMockSession({ title: 'My Important Session' })
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )
    expect(screen.getByText('My Important Session')).toBeInTheDocument()
    expect(screen.getByText('anthropic / claude-3-5-sonnet')).toBeInTheDocument()
  })

  it('shows fork point message when provided', () => {
    const session = createMockSession()
    const message = createMockMessage({ content: 'Fork at this message' })
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )
    expect(screen.getByText('assistant')).toBeInTheDocument()
    expect(screen.getByText('Fork at this message')).toBeInTheDocument()
  })

  it('shows message selector when messages are provided without forkPointMessage', () => {
    const session = createMockSession()
    const messages = [
      createMockMessage({ id: 'msg-1', role: 'user', content: 'First message' }),
      createMockMessage({ id: 'msg-2', role: 'assistant', content: 'Second message' }),
    ]
    render(
      <SessionForkDialog
        session={session}
        messages={messages}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )
    expect(screen.getByRole('combobox')).toBeInTheDocument()
    expect(screen.getByText(/Select a message/)).toBeInTheDocument()
  })

  it('allows setting a custom fork title', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    const titleInput = screen.getByPlaceholderText(/Fork of/)
    await user.clear(titleInput)
    await user.type(titleInput, 'My Custom Fork')

    await user.click(screen.getByText('Create Fork'))

    await waitFor(() => {
      expect(mockOnFork).toHaveBeenCalledWith(
        'session-123',
        'msg-123',
        expect.objectContaining({ title: 'My Custom Fork' })
      )
    })
  })

  it('allows toggling copy options', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    // Uncheck copy messages
    const copyMessagesCheckbox = screen.getByLabelText(/Copy messages to fork/i)
    await user.click(copyMessagesCheckbox)

    await user.click(screen.getByText('Create Fork'))

    await waitFor(() => {
      expect(mockOnFork).toHaveBeenCalledWith(
        'session-123',
        'msg-123',
        expect.objectContaining({
          copyMessages: false,
          copyToolCalls: false,
          copySummaries: false,
        })
      )
    })
  })

  it('calls onFork when Create Fork is clicked', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByText('Create Fork'))

    await waitFor(() => {
      expect(mockOnFork).toHaveBeenCalledWith(
        'session-123',
        'msg-123',
        expect.objectContaining({
          copyMessages: true,
          copyToolCalls: true,
          copySummaries: true,
        })
      )
    })
  })

  it('shows success state after successful fork', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByText('Create Fork'))

    // Wait for the success state title to appear
    await waitFor(() => {
      expect(screen.queryByText('Session Forked Successfully')).toBeInTheDocument()
    }, { timeout: 5000 })

    // Use findBy to wait for stats
    expect(await screen.findByText(/5 messages/)).toBeInTheDocument()
    expect(screen.getByText(/2 tool calls/)).toBeInTheDocument()
    expect(screen.getByText(/1 summaries/)).toBeInTheDocument()
  })

  it('shows error message when fork fails', async () => {
    mockOnFork.mockRejectedValue(new Error('Fork failed: session not found'))
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByText('Create Fork'))

    await waitFor(() => {
      expect(screen.queryByText('Fork failed: session not found')).toBeInTheDocument()
    }, { timeout: 5000 })
  })

  it('calls onNavigateToSession when Open Forked Session is clicked', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
        onNavigateToSession={mockOnNavigateToSession}
      />
    )

    await user.click(screen.getByText('Create Fork'))

    // Wait for success screen
    const navigateButton = await screen.findByText('Open Forked Session')
    
    await user.click(navigateButton)
    
    expect(mockOnNavigateToSession).toHaveBeenCalledWith('fork-session-456')
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('calls onClose when Cancel is clicked', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByText('Cancel'))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('calls onClose when close button is clicked', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByLabelText('Close dialog'))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('calls onClose when backdrop is clicked', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await user.click(screen.getByRole('dialog'))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('handles escape key to close dialog', () => {
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('does not close when loading and escape is pressed', () => {
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
        isLoading={true}
      />
    )

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(mockOnClose).not.toHaveBeenCalled()
  })

  it('disables Create Fork button when no fork point is selected', () => {
    const session = createMockSession()
    render(
      <SessionForkDialog
        session={session}
        messages={[]}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    const createButton = screen.getByText('Create Fork').closest('button')
    expect(createButton).toBeDisabled()
  })

  it('shows provider override options when providers are provided', () => {
    const session = createMockSession()
    const providers = createMockProviders()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        providers={providers}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    expect(screen.getByText(/Provider Override/)).toBeInTheDocument()
    expect(screen.getByText(/Keep current/)).toBeInTheDocument()
  })

  it('shows model selector when provider is overridden', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const providers = createMockProviders()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        providers={providers}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    // Find the provider select by its label
    const providerSelect = screen.getAllByRole('combobox')[0]
    await user.selectOptions(providerSelect, 'openai')

    // Model selector should appear
    expect(screen.getByText('GPT-4o')).toBeInTheDocument()
  })

  it('displays fork depth information in success state', async () => {
    mockOnFork.mockResolvedValue(createMockForkResult({ forkDepth: 3 }))
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await act(async () => {
      await user.click(screen.getByText('Create Fork'))
      await flushPromises()
    })

    await waitFor(() => {
      expect(screen.getByText(/Level 3/)).toBeInTheDocument()
    })
  })

  it('displays ancestor chain in success state', async () => {
    mockOnFork.mockResolvedValue(
      createMockForkResult({
        ancestorChain: ['root-session', 'parent-session', 'session-123'],
      })
    )
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    await act(async () => {
      await user.click(screen.getByText('Create Fork'))
      await flushPromises()
    })

    await waitFor(() => {
      expect(screen.getByText('Ancestor Chain')).toBeInTheDocument()
    })
  })

  it('shows loading state while forking', async () => {
    // Create a promise that we control
    let resolveForK: (value: ForkResult) => void
    mockOnFork.mockImplementation(
      () => new Promise<ForkResult>((resolve) => { resolveForK = resolve })
    )

    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    // Click to start forking - don't await to catch loading state
    const clickPromise = user.click(screen.getByText('Create Fork'))

    await waitFor(() => {
      // Should show forking state
      expect(screen.getByText('Forking...')).toBeInTheDocument()
    })

    // Resolve the fork
    await act(async () => {
      resolveForK!(createMockForkResult())
      await flushPromises()
    })

    await clickPromise

    await waitFor(() => {
      expect(screen.getByText('Session Forked Successfully')).toBeInTheDocument()
    })
  })

  it('resets state when dialog is reopened', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    const { rerender } = render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    // Complete a fork
    await act(async () => {
      await user.click(screen.getByText('Create Fork'))
      await flushPromises()
    })

    await waitFor(() => {
      expect(screen.getByText('Session Forked Successfully')).toBeInTheDocument()
    })

    // Close and reopen
    rerender(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={false}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    rerender(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
      />
    )

    // Should be back to form state
    expect(screen.getByText('Fork Session')).toBeInTheDocument()
    expect(screen.queryByText('Session Forked Successfully')).not.toBeInTheDocument()
  })
})
