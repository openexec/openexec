/**
 * Tests for RestartRequestList Component
 * @module components/chat/restart/__tests__/RestartRequestList
 */
import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RestartRequestList from '../RestartRequestList'
import type { RestartRequest, RestartStatus } from '../../../../types/restart'

const createMockRequest = (overrides: Partial<RestartRequest> = {}): RestartRequest => ({
  id: `req-${Math.random().toString(36).substr(2, 9)}`,
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

describe('RestartRequestList', () => {
  const mockOnRequestClick = vi.fn()
  const mockOnQuickApprove = vi.fn()
  const mockOnQuickReject = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the list title', () => {
    render(
      <RestartRequestList
        requests={[]}
        title="Test Requests"
      />
    )

    expect(screen.getByText('Test Requests')).toBeInTheDocument()
  })

  it('displays empty message when no requests', () => {
    render(
      <RestartRequestList
        requests={[]}
        emptyMessage="No requests found"
      />
    )

    expect(screen.getByText('No requests found')).toBeInTheDocument()
  })

  it('renders list of requests', () => {
    const requests = [
      createMockRequest({ id: 'req-1', reason: 'code_change' }),
      createMockRequest({ id: 'req-2', reason: 'config_change' }),
      createMockRequest({ id: 'req-3', reason: 'recovery' }),
    ]

    render(<RestartRequestList requests={requests} />)

    expect(screen.getByText('Code Change')).toBeInTheDocument()
    expect(screen.getByText('Config Change')).toBeInTheDocument()
    expect(screen.getByText('Recovery')).toBeInTheDocument()
  })

  it('displays correct request count', () => {
    const requests = [
      createMockRequest({ id: 'req-1' }),
      createMockRequest({ id: 'req-2' }),
      createMockRequest({ id: 'req-3' }),
    ]

    render(<RestartRequestList requests={requests} />)

    expect(screen.getByText('(3)')).toBeInTheDocument()
  })

  it('shows filter buttons when showFilters is true', () => {
    const requests = [createMockRequest({ status: 'pending' })]

    render(<RestartRequestList requests={requests} showFilters />)

    // Use getAllByRole to find filter buttons specifically
    expect(screen.getByRole('button', { name: /^All/ })).toBeInTheDocument()
    // "Pending" appears as both a status badge and filter button, so find by role
    const pendingButtons = screen.getAllByText(/Pending/)
    expect(pendingButtons.length).toBeGreaterThanOrEqual(1)
  })

  it('hides filter buttons when showFilters is false', () => {
    const requests = [createMockRequest({ status: 'pending' })]

    render(<RestartRequestList requests={requests} showFilters={false} />)

    expect(screen.queryByRole('button', { name: /All/ })).not.toBeInTheDocument()
  })

  it('filters requests by status when filter is clicked', async () => {
    const user = userEvent.setup()
    const requests = [
      createMockRequest({ id: 'req-1', status: 'pending', description: 'Pending request' }),
      createMockRequest({ id: 'req-2', status: 'approved', description: 'Approved request' }),
    ]

    render(<RestartRequestList requests={requests} showFilters />)

    // Initially both should be visible
    expect(screen.getByText('Pending request')).toBeInTheDocument()
    expect(screen.getByText('Approved request')).toBeInTheDocument()

    // Click pending filter
    await user.click(screen.getByRole('button', { name: /Pending/ }))

    // Only pending should be visible
    expect(screen.getByText('Pending request')).toBeInTheDocument()
    expect(screen.queryByText('Approved request')).not.toBeInTheDocument()
  })

  it('shows all requests when "All" filter is clicked', async () => {
    const user = userEvent.setup()
    const requests = [
      createMockRequest({ id: 'req-1', status: 'pending', description: 'Pending request' }),
      createMockRequest({ id: 'req-2', status: 'approved', description: 'Approved request' }),
    ]

    render(<RestartRequestList requests={requests} showFilters />)

    // Click pending filter first
    await user.click(screen.getByRole('button', { name: /Pending/ }))

    // Click all filter
    await user.click(screen.getByRole('button', { name: /All/ }))

    // Both should be visible
    expect(screen.getByText('Pending request')).toBeInTheDocument()
    expect(screen.getByText('Approved request')).toBeInTheDocument()
  })

  it('calls onRequestClick when a request card is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ id: 'req-1' })

    render(
      <RestartRequestList
        requests={[request]}
        onRequestClick={mockOnRequestClick}
      />
    )

    await user.click(screen.getByText('Code Change'))
    expect(mockOnRequestClick).toHaveBeenCalledWith(request)
  })

  it('passes quick action handlers to request cards', () => {
    const request = createMockRequest({ id: 'req-1', status: 'pending' })

    render(
      <RestartRequestList
        requests={[request]}
        onQuickApprove={mockOnQuickApprove}
        onQuickReject={mockOnQuickReject}
      />
    )

    // Quick action buttons should be visible for pending requests
    expect(screen.getByText('Approve')).toBeInTheDocument()
    expect(screen.getByText('Reject')).toBeInTheDocument()
  })

  it('calls onQuickApprove when approve is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ id: 'req-1', status: 'pending' })

    render(
      <RestartRequestList
        requests={[request]}
        onQuickApprove={mockOnQuickApprove}
      />
    )

    await user.click(screen.getByText('Approve'))
    expect(mockOnQuickApprove).toHaveBeenCalledWith(request)
  })

  it('calls onQuickReject when reject is clicked', async () => {
    const user = userEvent.setup()
    const request = createMockRequest({ id: 'req-1', status: 'pending' })

    render(
      <RestartRequestList
        requests={[request]}
        onQuickReject={mockOnQuickReject}
      />
    )

    await user.click(screen.getByText('Reject'))
    expect(mockOnQuickReject).toHaveBeenCalledWith(request)
  })

  it('displays filter count badges', () => {
    const requests = [
      createMockRequest({ id: 'req-1', status: 'pending' }),
      createMockRequest({ id: 'req-2', status: 'pending' }),
      createMockRequest({ id: 'req-3', status: 'approved' }),
    ]

    render(<RestartRequestList requests={requests} showFilters />)

    // Should show counts in filter buttons
    // "All" should show 3, "Pending" should show 2, "Approved" should show 1
    const allButton = screen.getByRole('button', { name: /All/ })
    const pendingButton = screen.getByRole('button', { name: /Pending/ })
    const approvedButton = screen.getByRole('button', { name: /Approved/ })

    expect(allButton).toHaveTextContent('3')
    expect(pendingButton).toHaveTextContent('2')
    expect(approvedButton).toHaveTextContent('1')
  })

  it('only shows filter options for statuses that have requests', () => {
    const requests = [
      createMockRequest({ id: 'req-1', status: 'pending' }),
      createMockRequest({ id: 'req-2', status: 'approved' }),
    ]

    render(<RestartRequestList requests={requests} showFilters />)

    // Should show All, Pending, and Approved
    expect(screen.getByRole('button', { name: /All/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Pending/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Approved/ })).toBeInTheDocument()

    // Should not show statuses with 0 count
    expect(screen.queryByRole('button', { name: /Rejected/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Failed/ })).not.toBeInTheDocument()
  })

  it('updates count when filter is active', async () => {
    const user = userEvent.setup()
    const requests = [
      createMockRequest({ id: 'req-1', status: 'pending' }),
      createMockRequest({ id: 'req-2', status: 'approved' }),
    ]

    render(<RestartRequestList requests={requests} showFilters />)

    // Initially shows 2 total
    expect(screen.getByText('(2)')).toBeInTheDocument()

    // Filter to pending
    await user.click(screen.getByRole('button', { name: /Pending/ }))

    // Should show 1 for filtered count
    expect(screen.getByText('(1)')).toBeInTheDocument()
  })
})
