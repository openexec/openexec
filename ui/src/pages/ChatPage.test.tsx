import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import ChatPage from './ChatPage'
import { STAGE_INFO } from '../types/blueprint'

// Only network boundaries are substituted. All hooks and components are real.
class Socket {
  static OPEN = 1
  static latest: Socket
  readyState = 1
  onmessage: ((event: MessageEvent) => void) | null = null
  send = vi.fn()
  close = vi.fn()
  constructor() { Socket.latest = this }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('shows D6 stages from WebSocket messages through ChatPage without replaying events', async () => {
  // Turn React's otherwise unbounded effect-loop warning into a test failure.
  const reportError = console.error
  vi.spyOn(console, 'error').mockImplementation((...args) => {
    if (String(args[0]).includes('Maximum update depth exceeded')) {
      throw new Error('ChatPage replayed a stage event in an effect loop')
    }
    reportError(...args)
  })
  Element.prototype.scrollIntoView = vi.fn()
  vi.stubGlobal('WebSocket', Socket)
  vi.stubGlobal('fetch', vi.fn(async (url: string) => ({
    ok: true,
    json: async () => url.includes('approvals') ? { approvals: [] } : [],
  })))
  const config = { apiUrl: 'http://test/api', wsUrl: 'ws://test/ws' }
  const view = render(<ChatPage config={config} />)
  await act(async () => {})
  fireEvent.click(screen.getByRole('button', { name: 'Open panel' }))
  let sequence = 0
  const send = (type: string, name: string, extra = {}) => act(() => {
    Socket.latest.onmessage?.(new MessageEvent('message', { data: JSON.stringify({
      type: 'event', payload: {
        id: `event-${++sequence}`, type, kind: 'stage',
        timestamp: `2026-10-18T10:00:${String(sequence).padStart(2, '0')}Z`,
        metadata: { stage_name: name, attempt: 2 }, ...extra,
      },
    }) }))
  })
  const stage = (name: keyof typeof STAGE_INFO) =>
    screen.getByText(STAGE_INFO[name].label).closest('.stage-timeline__stage') as HTMLElement

  for (const name of ['fix_lint', 'production_readiness_check'] as const) {
    send('stage_start', name)
    expect(stage(name)).toHaveClass('stage-timeline__stage--running')
    send('stage_failed', name, { error: 'check refused' })
    expect(stage(name)).toHaveClass('stage-timeline__stage--failed')
    expect(within(stage(name)).getByTitle('check refused')).toBeInTheDocument()
    send('stage_retry', name)
    expect(within(stage(name)).getByText('Attempt 2')).toBeInTheDocument()
    send('stage_start', name)
    send('stage_complete', name)
    expect(stage(name)).toHaveClass('stage-timeline__stage--completed')
    expect(within(stage(name)).getByText('1.0s')).toBeInTheDocument()
  }
  send('stage_start', 'unknown_stage')
  expect(document.querySelectorAll('.stage-timeline__stage--running')).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Close panel' }))
  fireEvent.click(screen.getByRole('button', { name: 'Open panel' }))
  view.rerender(<ChatPage config={{ ...config }} />)
  expect(stage('fix_lint')).toHaveClass('stage-timeline__stage--completed')
  expect(stage('production_readiness_check')).toHaveClass('stage-timeline__stage--completed')
  view.unmount()
  // Timeline state is transient; remount must not resurrect a previous run.
  render(<ChatPage config={config} />)
  await act(async () => {})
  fireEvent.click(screen.getByRole('button', { name: 'Open panel' }))
  expect(document.querySelector('.stage-timeline')).not.toBeInTheDocument()
})
