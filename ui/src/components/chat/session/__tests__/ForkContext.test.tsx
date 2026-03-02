/**
 * ForkContext Tests
 *
 * Tests for the ForkProvider and useForkContext hook.
 */

import React from 'react'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ForkProvider, useForkContext, useForkContextOptional } from '../ForkContext'
import type { Session, Message } from '../../../../types/chat'

// Mock the useFork hook
vi.mock('../../../../hooks/useFork', () => ({
  useFork: vi.fn(() => ({
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
    getForkInfo: vi.fn(),
    listSessionForks: vi.fn(),
    clearForkError: vi.fn(),
  })),
}))

// Import the mocked hook for assertions
import { useFork } from '../../../../hooks/useFork'
const mockUseFork = useFork as Mock

// Mock session
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

// Mock message
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

// Test component that uses the context
const TestConsumer: React.FC = () => {
  const context = useForkContext()
  return (
    <div>
      <span data-testid="is-open">{context.isForkDialogOpen.toString()}</span>
      <span data-testid="is-forking">{context.isForking.toString()}</span>
      <button
        data-testid="open-dialog"
        onClick={() => context.openForkDialog(mockSession)}
      >
        Open
      </button>
      <button data-testid="close-dialog" onClick={context.closeForkDialog}>
        Close
      </button>
    </div>
  )
}

// Optional context test component
const OptionalTestConsumer: React.FC = () => {
  const context = useForkContextOptional()
  return (
    <div>
      <span data-testid="has-context">{(context !== undefined).toString()}</span>
    </div>
  )
}

describe('ForkContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('ForkProvider', () => {
    it('renders children', () => {
      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <div data-testid="child">Child content</div>
        </ForkProvider>
      )

      expect(screen.getByTestId('child')).toBeInTheDocument()
    })

    it('provides context values to children', () => {
      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      expect(screen.getByTestId('is-open')).toHaveTextContent('false')
      expect(screen.getByTestId('is-forking')).toHaveTextContent('false')
    })

    it('passes apiBaseUrl and authToken to useFork hook', () => {
      render(
        <ForkProvider apiBaseUrl="http://test.com" authToken="test-token">
          <TestConsumer />
        </ForkProvider>
      )

      expect(mockUseFork).toHaveBeenCalledWith({
        baseUrl: 'http://test.com',
        authToken: 'test-token',
      })
    })

    it('calls openForkDialog when triggered', () => {
      const mockOpenForkDialog = vi.fn()
      mockUseFork.mockReturnValue({
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
        openForkDialog: mockOpenForkDialog,
        closeForkDialog: vi.fn(),
        forkSession: vi.fn(),
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      fireEvent.click(screen.getByTestId('open-dialog'))
      expect(mockOpenForkDialog).toHaveBeenCalledWith(mockSession)
    })

    it('calls closeForkDialog when triggered', () => {
      const mockCloseForkDialog = vi.fn()
      mockUseFork.mockReturnValue({
        isForkDialogOpen: true,
        sessionToFork: mockSession,
        forkPointMessage: undefined,
        isForking: false,
        forkError: undefined,
        lastForkResult: undefined,
        forkInfo: undefined,
        forkInfoLoading: false,
        sessionForks: [],
        sessionForksLoading: false,
        openForkDialog: vi.fn(),
        closeForkDialog: mockCloseForkDialog,
        forkSession: vi.fn(),
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      fireEvent.click(screen.getByTestId('close-dialog'))
      expect(mockCloseForkDialog).toHaveBeenCalled()
    })
  })

  describe('useForkContext', () => {
    it('throws when used outside provider', () => {
      // Suppress console error for this test
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

      expect(() => {
        render(<TestConsumer />)
      }).toThrow('useForkContext must be used within a ForkProvider')

      consoleSpy.mockRestore()
    })

    it('returns context values when inside provider', () => {
      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      expect(screen.getByTestId('is-open')).toBeInTheDocument()
    })
  })

  describe('useForkContextOptional', () => {
    it('returns undefined when used outside provider', () => {
      render(<OptionalTestConsumer />)
      expect(screen.getByTestId('has-context')).toHaveTextContent('false')
    })

    it('returns context when inside provider', () => {
      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <OptionalTestConsumer />
        </ForkProvider>
      )
      expect(screen.getByTestId('has-context')).toHaveTextContent('true')
    })
  })

  describe('Fork dialog rendering', () => {
    it('renders SessionForkDialog when sessionToFork is set', () => {
      mockUseFork.mockReturnValue({
        isForkDialogOpen: true,
        sessionToFork: mockSession,
        forkPointMessage: mockMessage,
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
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      // The dialog should be rendered (check for dialog elements)
      expect(screen.getByRole('dialog')).toBeInTheDocument()
    })

    it('does not render dialog when sessionToFork is undefined', () => {
      mockUseFork.mockReturnValue({
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
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  describe('Context value updates', () => {
    it('reflects loading state from hook', () => {
      mockUseFork.mockReturnValue({
        isForkDialogOpen: false,
        sessionToFork: undefined,
        forkPointMessage: undefined,
        isForking: true,
        forkError: undefined,
        lastForkResult: undefined,
        forkInfo: undefined,
        forkInfoLoading: false,
        sessionForks: [],
        sessionForksLoading: false,
        openForkDialog: vi.fn(),
        closeForkDialog: vi.fn(),
        forkSession: vi.fn(),
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestConsumer />
        </ForkProvider>
      )

      expect(screen.getByTestId('is-forking')).toHaveTextContent('true')
    })

    it('reflects error state from hook', () => {
      const TestErrorConsumer: React.FC = () => {
        const context = useForkContext()
        return <span data-testid="error">{context.forkError || 'none'}</span>
      }

      mockUseFork.mockReturnValue({
        isForkDialogOpen: false,
        sessionToFork: undefined,
        forkPointMessage: undefined,
        isForking: false,
        forkError: 'Test error',
        lastForkResult: undefined,
        forkInfo: undefined,
        forkInfoLoading: false,
        sessionForks: [],
        sessionForksLoading: false,
        openForkDialog: vi.fn(),
        closeForkDialog: vi.fn(),
        forkSession: vi.fn(),
        getForkInfo: vi.fn(),
        listSessionForks: vi.fn(),
        clearForkError: vi.fn(),
      })

      render(
        <ForkProvider apiBaseUrl="http://test.com">
          <TestErrorConsumer />
        </ForkProvider>
      )

      expect(screen.getByTestId('error')).toHaveTextContent('Test error')
    })
  })
})
