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
    expect(screen.getAllByText(/Fork Session/i).length).toBeGreaterThan(0)
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
    expect(screen.getAllByText(/assistant/i).length).toBeGreaterThan(0)
    expect(screen.getByText('Fork at this message')).toBeInTheDocument()
  })

  it('allows setting a custom fork title', async () => {
    const user = userEvent.setup()
    const session = createMockSession({ title: 'Test Session' })
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

    const titleInput = screen.getByPlaceholderText(/Fork of "Test Session"/i)
    await user.clear(titleInput)
    await user.type(titleInput, 'My Custom Fork')

    await user.click(screen.getByText(/Create Fork/i))

    await waitFor(() => {
      expect(mockOnFork).toHaveBeenCalledWith(
        'session-123',
        'msg-123',
        expect.objectContaining({ title: 'My Custom Fork' })
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

    await user.click(screen.getByText(/Create Fork/i))

    // Wait for the success state title to appear
    await waitFor(() => {
      expect(screen.getByText('Session Forked Successfully')).toBeInTheDocument()
    }, { timeout: 5000 })

    // Use findBy to wait for stats
    expect(await screen.findByText(/5 messages/i)).toBeInTheDocument()
    expect(screen.getByText(/2 tool calls/i)).toBeInTheDocument()
    expect(screen.getByText(/1 summaries/i)).toBeInTheDocument()
  })

  it('shows loading state while forking', async () => {
    // Create a promise that we control
    let resolveFork: (value: ForkResult) => void
    mockOnFork.mockImplementation(
      () => new Promise<ForkResult>((resolve) => { resolveFork = resolve })
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
        isLoading={true}
      />
    )

    // Specifically find the text in the button, not the info banner
    const forkingElements = screen.getAllByText(/Forking.../i)
    expect(forkingElements.length).toBeGreaterThan(0)
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

    await user.click(screen.getByText(/Create Fork/i))

    await waitFor(() => {
      // Specifically find the label in the details section
      expect(screen.getByText('Ancestor Chain')).toBeInTheDocument()
    }, { timeout: 5000 })
  })
})
