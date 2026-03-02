/**
 * Tests for RestartBanner Component
 * @module components/chat/restart/__tests__/RestartBanner
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RestartBanner from '../RestartBanner'
import type { RestartRequest, RestartReason } from '../../../../types/restart'

const createMockRequest = (overrides: Partial<RestartRequest> = {}): RestartRequest => ({
  id: 'req-123',
  reason: 'code_change',
  description: 'Code changes detected',
  requestedBy: 'agent-456',
  status: 'pending',
  buildRequired: false,
  port: 8080,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  resumeEnabled: false,
  resumeOnStartup: false,
  ...overrides,
})

describe('RestartBanner', () => {
  const mockOnApprove = vi.fn()
  const mockOnReject = vi.fn()
  const mockOnViewDetails = vi.fn()
  const mockOnDismiss = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the restart banner with title', () => {
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText('Restart Approval Required')).toBeInTheDocument()
  })

  it('displays appropriate message for code_change reason', () => {
    const request = createMockRequest({ reason: 'code_change' })
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText(/Code changes require an orchestrator restart/)).toBeInTheDocument()
  })

  it('displays appropriate message for config_change reason', () => {
    const request = createMockRequest({ reason: 'config_change' })
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText(/Configuration changes require an orchestrator restart/)).toBeInTheDocument()
  })

  it('mentions build requirement when buildRequired is true', () => {
    const request = createMockRequest({ buildRequired: true })
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText(/A build will run before restart/)).toBeInTheDocument()
  })

  it('mentions session resume when resumeEnabled is true', () => {
    const request = createMockRequest({ resumeEnabled: true })
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText(/Your session will resume automatically/)).toBeInTheDocument()
  })

  it('calls onApprove when approve button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    await user.click(screen.getByText('Approve'))
    expect(mockOnApprove).toHaveBeenCalled()
  })

  it('calls onReject when reject button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    await user.click(screen.getByText('Reject'))
    expect(mockOnReject).toHaveBeenCalled()
  })

  it('shows details button when onViewDetails is provided', () => {
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onViewDetails={mockOnViewDetails}
      />
    )

    expect(screen.getByText('Details')).toBeInTheDocument()
  })

  it('calls onViewDetails when details button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onViewDetails={mockOnViewDetails}
      />
    )

    await user.click(screen.getByText('Details'))
    expect(mockOnViewDetails).toHaveBeenCalled()
  })

  it('shows dismiss button when dismissible and onDismiss provided', () => {
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onDismiss={mockOnDismiss}
        dismissible
      />
    )

    expect(screen.getByLabelText('Dismiss notification')).toBeInTheDocument()
  })

  it('calls onDismiss when dismiss button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onDismiss={mockOnDismiss}
        dismissible
      />
    )

    await user.click(screen.getByLabelText('Dismiss notification'))
    expect(mockOnDismiss).toHaveBeenCalled()
  })

  it('does not show dismiss button when dismissible is false', () => {
    const request = createMockRequest()
    render(
      <RestartBanner
        request={request}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onDismiss={mockOnDismiss}
        dismissible={false}
      />
    )

    expect(screen.queryByLabelText('Dismiss notification')).not.toBeInTheDocument()
  })

  it('displays correct messages for all restart reasons', () => {
    const reasons: { reason: RestartReason; expectedText: string }[] = [
      { reason: 'code_change', expectedText: /Code changes require/ },
      { reason: 'config_change', expectedText: /Configuration changes require/ },
      { reason: 'user_requested', expectedText: /A manual restart/ },
      { reason: 'recovery', expectedText: /Error recovery require/ },
      { reason: 'upgrade', expectedText: /An upgrade require/ },
    ]

    reasons.forEach(({ reason, expectedText }) => {
      const request = createMockRequest({ reason })
      const { unmount } = render(
        <RestartBanner
          request={request}
          onApprove={mockOnApprove}
          onReject={mockOnReject}
        />
      )
      expect(screen.getByText(expectedText)).toBeInTheDocument()
      unmount()
    })
  })
})
