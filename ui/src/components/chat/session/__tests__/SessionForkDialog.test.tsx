/**
 * Tests for SessionForkDialog Component
 * @module components/chat/session/__tests__/SessionForkDialog
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SessionForkDialog from '../SessionForkDialog'
import type { Session, Message } from '../../../../types/chat'
import type { ForkResult } from '../SessionForkDialog'

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

describe('SessionForkDialog', () => {
  const mockOnClose = vi.fn()
  const mockOnFork = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
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
    expect(screen.getByText('Fork at this message')).toBeInTheDocument()
  })

  it('allows setting a custom fork title', async () => {
    const user = userEvent.setup()
    const session = createMockSession({ title: 'Test Session' })
    const message = createMockMessage()
    mockOnFork.mockResolvedValue(createMockForkResult({ title: 'My Custom Fork' }))

    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
        isLoading={false}
      />
    )

    const titleInput = screen.getByPlaceholderText(/Fork of "Test Session"/i)
    await user.clear(titleInput)
    await user.type(titleInput, 'My Custom Fork')

    const forkButton = screen.getByRole('button', { name: /Create Fork/i })
    await act(async () => {
      await user.click(forkButton)
    })

    await waitFor(() => {
      expect(mockOnFork).toHaveBeenCalledWith(
        'session-123',
        'msg-123',
        expect.objectContaining({ title: 'My Custom Fork' })
      )
    }, { timeout: 3000 })
  })

  it('shows success state after successful fork', async () => {
    const user = userEvent.setup()
    const session = createMockSession()
    const message = createMockMessage()
    mockOnFork.mockResolvedValue(createMockForkResult())

    render(
      <SessionForkDialog
        session={session}
        forkPointMessage={message}
        isOpen={true}
        onClose={mockOnClose}
        onFork={mockOnFork}
        isLoading={false}
      />
    )

    const forkButton = screen.getByRole('button', { name: /Create Fork/i })
    await act(async () => {
      await user.click(forkButton)
    })

    const successTitle = await screen.findByText('Session Forked Successfully', {}, { timeout: 3000 })
    expect(successTitle).toBeInTheDocument()
    expect(screen.getByText(/Created new session/i)).toBeInTheDocument()
  })

  it('shows loading state while forking', async () => {
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
      />
    )

    const forkButton = screen.getByRole('button', { name: /Create Fork/i })
    await act(async () => {
      await user.click(forkButton)
    })
    expect(screen.getByText(/Forking.../i)).toBeInTheDocument()
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
        isLoading={false}
      />
    )

    const forkButton = screen.getByRole('button', { name: /Create Fork/i })
    await act(async () => {
      await user.click(forkButton)
    })
    
    // The "Ancestor Chain" label appears in the success state
    await screen.findByText('Ancestor Chain')
    expect(screen.getByText('root-ses...')).toBeInTheDocument()
  })
})
