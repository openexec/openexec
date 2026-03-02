/**
 * Tests for RestartApprovalDialog Component
 * @module components/chat/restart/__tests__/RestartApprovalDialog
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RestartApprovalDialog from '../RestartApprovalDialog'
import type { RestartRequest } from '../../../../types/restart'

const createMockRequest = (overrides: Partial<RestartRequest> = {}): RestartRequest => ({
  id: 'req-123',
  reason: 'code_change',
  description: 'Code changes detected in main.go',
  requestedBy: 'agent-456',
  sessionId: 'session-789',
  status: 'pending',
  buildRequired: true,
  port: 8080,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  resumeEnabled: true,
  resumeOnStartup: true,
  ...overrides,
})

describe('RestartApprovalDialog', () => {
  const mockOnClose = vi.fn()
  const mockOnApprove = vi.fn()
  const mockOnReject = vi.fn()
  const mockOnCancel = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when isOpen is false', () => {
    const request = createMockRequest()
    const { container } = render(
      <RestartApprovalDialog
        request={request}
        isOpen={false}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders dialog when isOpen is true', () => {
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Restart Approval Required')).toBeInTheDocument()
  })

  it('displays the restart request details', () => {
    const request = createMockRequest({
      reason: 'code_change',
      description: 'Test description',
      requestedBy: 'test-agent',
    })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText('Code Change')).toBeInTheDocument()
    expect(screen.getByText('Test description')).toBeInTheDocument()
    expect(screen.getByText('test-agent')).toBeInTheDocument()
  })

  it('shows build required indicator when buildRequired is true', () => {
    const request = createMockRequest({ buildRequired: true })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )
    expect(screen.getByText(/will rebuild before restart/)).toBeInTheDocument()
  })

  it('shows session resume status when resumeEnabled is true', () => {
    const request = createMockRequest({ resumeEnabled: true, resumeOnStartup: true })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )
    expect(screen.getByText(/auto-resume on startup/)).toBeInTheDocument()
  })

  it('calls onApprove when approve button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    await user.click(screen.getByText('Approve Restart'))
    expect(mockOnApprove).toHaveBeenCalledWith('req-123')
  })

  it('shows reject reason input when reject is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    // First click shows the input
    await user.click(screen.getByText('Reject'))
    expect(screen.getByPlaceholderText(/Provide a reason/)).toBeInTheDocument()
    expect(screen.getByText('Confirm Reject')).toBeInTheDocument()
  })

  it('calls onReject with reason when confirm reject is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    // Click reject to show input
    await user.click(screen.getByText('Reject'))

    // Type a reason
    const textarea = screen.getByPlaceholderText(/Provide a reason/)
    await user.type(textarea, 'Not safe to restart now')

    // Confirm rejection
    await user.click(screen.getByText('Confirm Reject'))
    expect(mockOnReject).toHaveBeenCalledWith('req-123', 'Not safe to restart now')
  })

  it('calls onClose when close button is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    await user.click(screen.getByLabelText('Close dialog'))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('calls onClose when backdrop is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    // Click the backdrop (the dialog wrapper)
    await user.click(screen.getByRole('dialog'))
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('shows cancel button when onCancel is provided and request is pending', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ status: 'pending' })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        onCancel={mockOnCancel}
      />
    )

    expect(screen.getByText('Cancel Request')).toBeInTheDocument()
    await user.click(screen.getByText('Cancel Request'))
    expect(mockOnCancel).toHaveBeenCalledWith('req-123')
  })

  it('disables buttons when isLoading is true', () => {
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        isLoading={true}
      />
    )

    // When loading, the approve button shows a spinner, so we find it by its class/role
    const buttons = screen.getAllByRole('button')
    const approveButton = buttons.find(btn => btn.className.includes('approve'))
    const rejectButton = buttons.find(btn => btn.className.includes('reject'))

    expect(approveButton).toBeDisabled()
    expect(rejectButton).toBeDisabled()
  })

  it('disables approve/reject for non-pending requests', () => {
    const request = createMockRequest({ status: 'approved' })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    expect(screen.getByText('Approve Restart').closest('button')).toBeDisabled()
    expect(screen.getByText('Reject').closest('button')).toBeDisabled()
  })

  it('displays different restart reasons correctly', () => {
    const reasons = [
      { reason: 'code_change', label: 'Code Change' },
      { reason: 'config_change', label: 'Configuration Change' },
      { reason: 'user_requested', label: 'User Requested' },
      { reason: 'recovery', label: 'Recovery' },
      { reason: 'upgrade', label: 'Upgrade' },
    ] as const

    reasons.forEach(({ reason, label }) => {
      const request = createMockRequest({ reason })
      const { unmount } = render(
        <RestartApprovalDialog
          request={request}
          isOpen={true}
          onClose={mockOnClose}
          onApprove={mockOnApprove}
          onReject={mockOnReject}
        />
      )
      expect(screen.getByText(label)).toBeInTheDocument()
      unmount()
    })
  })

  it('displays session ID when provided', () => {
    const request = createMockRequest({ sessionId: 'session-abc-123' })
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )
    expect(screen.getByText('session-abc-123')).toBeInTheDocument()
  })

  it('handles escape key to close dialog', () => {
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
      />
    )

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(mockOnClose).toHaveBeenCalled()
  })

  it('does not close when loading and escape is pressed', () => {
    const request = createMockRequest()
    render(
      <RestartApprovalDialog
        request={request}
        isOpen={true}
        onClose={mockOnClose}
        onApprove={mockOnApprove}
        onReject={mockOnReject}
        isLoading={true}
      />
    )

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(mockOnClose).not.toHaveBeenCalled()
  })
})
