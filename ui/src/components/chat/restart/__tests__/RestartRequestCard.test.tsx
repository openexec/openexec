/**
 * Tests for RestartRequestCard Component
 * @module components/chat/restart/__tests__/RestartRequestCard
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RestartRequestCard from '../RestartRequestCard'
import type { RestartRequest, RestartReason, RestartStatus } from '../../../../types/restart'

const createMockRequest = (overrides: Partial<RestartRequest> = {}): RestartRequest => ({
  id: 'req-123',
  reason: 'code_change',
  description: 'Code changes detected',
  requestedBy: 'agent-456',
  sessionId: 'session-789',
  status: 'pending',
  buildRequired: false,
  port: 8080,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  resumeEnabled: false,
  resumeOnStartup: false,
  ...overrides,
})

describe('RestartRequestCard', () => {
  const mockOnClick = vi.fn()
  const mockOnQuickApprove = vi.fn()
  const mockOnQuickReject = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the restart request basic information', () => {
    const request = createMockRequest()
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('Code Change')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
  })

  it('displays description when provided', () => {
    const request = createMockRequest({ description: 'Test description' })
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('Test description')).toBeInTheDocument()
  })

  it('hides description in compact mode', () => {
    const request = createMockRequest({ description: 'Test description' })
    render(<RestartRequestCard request={request} compact />)

    expect(screen.queryByText('Test description')).not.toBeInTheDocument()
  })

  it('displays requested by information', () => {
    const request = createMockRequest({ requestedBy: 'test-user' })
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('test-user')).toBeInTheDocument()
  })

  it('displays build required indicator when true', () => {
    const request = createMockRequest({ buildRequired: true })
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('Build required')).toBeInTheDocument()
  })

  it('displays resume enabled indicator when true', () => {
    const request = createMockRequest({ resumeEnabled: true })
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('Resume enabled')).toBeInTheDocument()
  })

  it('calls onClick when card is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(<RestartRequestCard request={request} onClick={mockOnClick} />)

    await user.click(screen.getByText('Code Change'))
    expect(mockOnClick).toHaveBeenCalled()
  })

  it('shows quick action buttons for pending requests', () => {
    const request = createMockRequest({ status: 'pending' })
    render(
      <RestartRequestCard
        request={request}
        onQuickApprove={mockOnQuickApprove}
        onQuickReject={mockOnQuickReject}
      />
    )

    expect(screen.getByText('Approve')).toBeInTheDocument()
    expect(screen.getByText('Reject')).toBeInTheDocument()
  })

  it('does not show quick action buttons for non-pending requests', () => {
    const request = createMockRequest({ status: 'approved' })
    render(
      <RestartRequestCard
        request={request}
        onQuickApprove={mockOnQuickApprove}
        onQuickReject={mockOnQuickReject}
      />
    )

    expect(screen.queryByText('Approve')).not.toBeInTheDocument()
    expect(screen.queryByText('Reject')).not.toBeInTheDocument()
  })

  it('calls onQuickApprove when approve button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ status: 'pending' })
    render(
      <RestartRequestCard
        request={request}
        onClick={mockOnClick}
        onQuickApprove={mockOnQuickApprove}
      />
    )

    await user.click(screen.getByText('Approve'))
    expect(mockOnQuickApprove).toHaveBeenCalled()
    // Should not trigger onClick
    expect(mockOnClick).not.toHaveBeenCalled()
  })

  it('calls onQuickReject when reject button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ status: 'pending' })
    render(
      <RestartRequestCard
        request={request}
        onClick={mockOnClick}
        onQuickReject={mockOnQuickReject}
      />
    )

    await user.click(screen.getByText('Reject'))
    expect(mockOnQuickReject).toHaveBeenCalled()
    // Should not trigger onClick
    expect(mockOnClick).not.toHaveBeenCalled()
  })

  it('displays different statuses with appropriate styling', () => {
    const statuses: RestartStatus[] = [
      'pending',
      'approved',
      'rejected',
      'in_progress',
      'complete',
      'failed',
      'cancelled',
    ]

    const labels: Record<RestartStatus, string> = {
      pending: 'Pending',
      approved: 'Approved',
      rejected: 'Rejected',
      in_progress: 'In Progress',
      complete: 'Complete',
      failed: 'Failed',
      cancelled: 'Cancelled',
    }

    statuses.forEach((status) => {
      const request = createMockRequest({ status })
      const { unmount } = render(<RestartRequestCard request={request} />)
      expect(screen.getByText(labels[status])).toBeInTheDocument()
      unmount()
    })
  })

  it('displays different restart reasons correctly', () => {
    const reasons: { reason: RestartReason; label: string }[] = [
      { reason: 'code_change', label: 'Code Change' },
      { reason: 'config_change', label: 'Config Change' },
      { reason: 'user_requested', label: 'User Request' },
      { reason: 'recovery', label: 'Recovery' },
      { reason: 'upgrade', label: 'Upgrade' },
    ]

    reasons.forEach(({ reason, label }) => {
      const request = createMockRequest({ reason })
      const { unmount } = render(<RestartRequestCard request={request} />)
      expect(screen.getByText(label)).toBeInTheDocument()
      unmount()
    })
  })

  it('supports keyboard navigation when onClick is provided', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(<RestartRequestCard request={request} onClick={mockOnClick} />)

    const card = screen.getByRole('button')
    card.focus()
    await user.keyboard('{Enter}')
    expect(mockOnClick).toHaveBeenCalled()
  })

  it('displays relative time', () => {
    const request = createMockRequest({
      createdAt: new Date().toISOString(),
    })
    render(<RestartRequestCard request={request} />)

    expect(screen.getByText('just now')).toBeInTheDocument()
  })
})
