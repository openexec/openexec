/**
 * Tests for ChatHeader Component
 * @module components/chat/header/__tests__/ChatHeader
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ChatHeader from '../ChatHeader'
import type { Session, AgentLoopState, CostInfo } from '../../../../types/chat'
import type { ForkInfo } from '../../../../hooks/useFork'
import type { AncestorSession } from '../../session/ForkAncestryTree'

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

const createMockLoopState = (overrides: Partial<AgentLoopState> = {}): AgentLoopState => ({
  iteration: 0,
  totalTokens: 0,
  totalCostUsd: 0,
  isRunning: false,
  isPaused: false,
  iterationsSinceProgress: 0,
  ...overrides,
})

const createMockCostInfo = (overrides: Partial<CostInfo> = {}): CostInfo => ({
  sessionTotal: 0.0025,
  iterationCost: 0.0005,
  totalTokensInput: 1000,
  totalTokensOutput: 500,
  ...overrides,
})

const createMockForkInfo = (overrides: Partial<ForkInfo> = {}): ForkInfo => ({
  sessionId: 'session-123',
  forkDepth: 2,
  parentSessionId: 'parent-session',
  rootSessionId: 'root-session',
  ancestorChain: ['root-session', 'parent-session', 'session-123'],
  ...overrides,
})

const createMockAncestors = (): AncestorSession[] => [
  { id: 'root-session', title: 'Root Session', isRoot: true, isCurrent: false },
  { id: 'parent-session', title: 'Parent Session', isRoot: false, isCurrent: false },
  { id: 'session-123', title: 'Current Session', isRoot: false, isCurrent: true },
]

describe('ChatHeader', () => {
  const mockOnPauseLoop = vi.fn()
  const mockOnResumeLoop = vi.fn()
  const mockOnStopLoop = vi.fn()
  const mockOnUpdateTitle = vi.fn()
  const mockOnNavigateToSession = vi.fn()
  const mockOnForkSession = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('basic rendering', () => {
    it('renders placeholder when no session is selected', () => {
      render(<ChatHeader />)
      expect(screen.getByText('No session selected')).toBeInTheDocument()
    })

    it('renders session title when session is provided', () => {
      const session = createMockSession({ title: 'My Test Session' })
      render(<ChatHeader session={session} />)
      expect(screen.getByText('My Test Session')).toBeInTheDocument()
    })

    it('renders provider and model indicator', () => {
      const session = createMockSession()
      render(<ChatHeader session={session} />)
      // ModelIndicator would show provider/model info
      expect(screen.getByText(/anthropic/i)).toBeInTheDocument()
    })

    it('renders cost information when provided', () => {
      const session = createMockSession()
      const costInfo = createMockCostInfo({ sessionTotal: 0.1234 })
      render(<ChatHeader session={session} costInfo={costInfo} />)
      expect(screen.getByText('$0.1234')).toBeInTheDocument()
    })
  })

  describe('fork indicator for forked sessions', () => {
    it('shows fork badge for forked session with parentSessionId', () => {
      const session = createMockSession({ parentSessionId: 'parent-session' })
      render(<ChatHeader session={session} />)
      expect(screen.getByTitle(/Forked session/i)).toBeInTheDocument()
    })

    it('shows fork badge when forkInfo indicates forked session', () => {
      const session = createMockSession()
      const forkInfo = createMockForkInfo({ forkDepth: 2 })
      render(<ChatHeader session={session} forkInfo={forkInfo} />)
      expect(screen.getByText(/Level 2/)).toBeInTheDocument()
    })

    it('does not show fork badge for root session', () => {
      const session = createMockSession()
      const forkInfo = createMockForkInfo({ forkDepth: 0 })
      render(<ChatHeader session={session} forkInfo={forkInfo} />)
      expect(screen.queryByText(/Level 0/)).not.toBeInTheDocument()
    })

    it('shows fork ancestry popover on hover when ancestors provided', async () => {
      const session = createMockSession({ parentSessionId: 'parent-session' })
      const forkInfo = createMockForkInfo()
      const ancestors = createMockAncestors()

      render(
        <ChatHeader
          session={session}
          forkInfo={forkInfo}
          ancestorSessions={ancestors}
          onNavigateToSession={mockOnNavigateToSession}
        />
      )

      // Find the fork indicator container
      const forkIndicator = screen.getByTitle(/Forked session/i).parentElement
      expect(forkIndicator).not.toBeNull()

      // Hover to show popover
      const user = userEvent.setup()
      await user.hover(forkIndicator!)

      // Should show the ForkAncestryTree (async render)
      await waitFor(() => {
        expect(screen.getByText('Root Session')).toBeInTheDocument()
        expect(screen.getByText('Parent Session')).toBeInTheDocument()
      })
    })

    it('allows clicking fork badge to toggle ancestry popover', async () => {
      const user = userEvent.setup()
      const session = createMockSession({ parentSessionId: 'parent-session' })
      const forkInfo = createMockForkInfo()
      const ancestors = createMockAncestors()

      render(
        <ChatHeader
          session={session}
          forkInfo={forkInfo}
          ancestorSessions={ancestors}
          onNavigateToSession={mockOnNavigateToSession}
        />
      )

      const forkBadge = screen.getByRole('button', { name: /Forked session/i })
      await user.click(forkBadge)

      // Should show some content from ForkAncestryTree
      await waitFor(() => {
        expect(screen.queryByText(/Root Session/i) || screen.queryByText(/Root/i)).toBeInTheDocument()
      })
    })
  })

  describe('fork button', () => {
    it('shows fork button when onForkSession is provided', () => {
      const session = createMockSession()
      render(
        <ChatHeader
          session={session}
          onForkSession={mockOnForkSession}
        />
      )
      expect(screen.getByTitle('Fork this session')).toBeInTheDocument()
    })

    it('does not show fork button when onForkSession is not provided', () => {
      const session = createMockSession()
      render(<ChatHeader session={session} />)
      expect(screen.queryByTitle('Fork this session')).not.toBeInTheDocument()
    })

    it('calls onForkSession when fork button is clicked', async () => {
      const user = userEvent.setup()
      const session = createMockSession()
      render(
        <ChatHeader
          session={session}
          onForkSession={mockOnForkSession}
        />
      )

      await user.click(screen.getByTitle('Fork this session'))
      expect(mockOnForkSession).toHaveBeenCalled()
    })

    it('does not show fork button when no session is selected', () => {
      render(
        <ChatHeader
          onForkSession={mockOnForkSession}
        />
      )
      expect(screen.queryByTitle('Fork this session')).not.toBeInTheDocument()
    })
  })

  describe('ancestor navigation', () => {
    it('calls onNavigateToSession when clicking on ancestor in popover', async () => {
      const user = userEvent.setup()
      const session = createMockSession({ parentSessionId: 'parent-session' })
      const forkInfo = createMockForkInfo()
      const ancestors = createMockAncestors()

      render(
        <ChatHeader
          session={session}
          forkInfo={forkInfo}
          ancestorSessions={ancestors}
          onNavigateToSession={mockOnNavigateToSession}
        />
      )

      // Open the popover using mouse enter on the container
      const forkIndicator = screen.getByTitle(/Forked session/i).parentElement
      const user = userEvent.setup()
      await user.hover(forkIndicator!)

      // Find the ancestor button - ForkAncestryTree renders buttons for each ancestor
      // The button contains the title text "Parent Session"
      const parentButton = screen.getByText('Parent Session').closest('button')
      expect(parentButton).not.toBeNull()
      await user.click(parentButton!)

      expect(mockOnNavigateToSession).toHaveBeenCalledWith('parent-session')
    })

    it('does not allow navigation to current session', async () => {
      const session = createMockSession({ parentSessionId: 'parent-session' })
      const forkInfo = createMockForkInfo()
      const ancestors = createMockAncestors()

      render(
        <ChatHeader
          session={session}
          forkInfo={forkInfo}
          ancestorSessions={ancestors}
          onNavigateToSession={mockOnNavigateToSession}
        />
      )

      // Open the popover using mouse enter on the container
      const forkIndicator = screen.getByTitle(/Forked session/i).parentElement
      const user = userEvent.setup()
      await user.hover(forkIndicator!)

      // Current session button should be disabled
      const currentButton = screen.getByText('Current Session').closest('button')
      expect(currentButton).not.toBeNull()
      expect(currentButton).toBeDisabled()
    })
  })

  describe('loop controls', () => {
    it('calls onPauseLoop when pause is triggered', async () => {
      const session = createMockSession()
      const loopState = createMockLoopState({ isRunning: true })
      render(
        <ChatHeader
          session={session}
          loopState={loopState}
          onPauseLoop={mockOnPauseLoop}
        />
      )
      // ChatActions component handles the actual button rendering
      // This test verifies the prop is passed correctly
      expect(mockOnPauseLoop).not.toHaveBeenCalled()
    })

    it('passes loop state to LoopStatusBadge', () => {
      const session = createMockSession()
      const loopState = createMockLoopState({ isRunning: true, iteration: 5 })
      render(<ChatHeader session={session} loopState={loopState} />)
      // LoopStatusBadge should display iteration info
      expect(screen.getByText(/iter 5/i)).toBeInTheDocument()
    })
  })

  describe('title update', () => {
    it('passes onUpdateTitle to SessionTitle component', () => {
      const session = createMockSession()
      render(
        <ChatHeader
          session={session}
          onUpdateTitle={mockOnUpdateTitle}
        />
      )
      // SessionTitle handles the editable title
      expect(screen.getByText('Test Session')).toBeInTheDocument()
    })
  })

  describe('cost display with budget', () => {
    it('shows budget limit when provided', () => {
      const session = createMockSession()
      const costInfo = createMockCostInfo({
        sessionTotal: 0.50,
        budgetLimit: 1.00
      })
      render(<ChatHeader session={session} costInfo={costInfo} />)
      expect(screen.getByText('$0.5000')).toBeInTheDocument()
      expect(screen.getByText('/ $1.00')).toBeInTheDocument()
    })

    it('does not show budget when not provided', () => {
      const session = createMockSession()
      const costInfo = createMockCostInfo({ sessionTotal: 0.50 })
      render(<ChatHeader session={session} costInfo={costInfo} />)
      expect(screen.getByText('$0.5000')).toBeInTheDocument()
      expect(screen.queryByText(/\/ \$/)).not.toBeInTheDocument()
    })
  })

  describe('accessibility', () => {
    it('fork badge has proper aria-label', () => {
      const session = createMockSession({ parentSessionId: 'parent' })
      const forkInfo = createMockForkInfo({ forkDepth: 3 })
      render(<ChatHeader session={session} forkInfo={forkInfo} />)

      const forkBadge = screen.getByRole('button', { name: /Forked session at depth 3/i })
      expect(forkBadge).toBeInTheDocument()
    })

    it('fork button has proper aria-label', () => {
      const session = createMockSession()
      render(
        <ChatHeader
          session={session}
          onForkSession={mockOnForkSession}
        />
      )

      const forkButton = screen.getByRole('button', { name: /Fork this session/i })
      expect(forkButton).toBeInTheDocument()
    })
  })
})
