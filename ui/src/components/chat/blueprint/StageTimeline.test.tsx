import { act, render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useBlueprint, type UseBlueprintReturn } from '../../../hooks/useBlueprint'
import type { LoopEvent } from '../../../types/chat'
import { BLUEPRINT_STAGES, STAGE_INFO } from '../../../types/blueprint'
import StageTimeline from './StageTimeline'

describe('default blueprint timeline events', () => {
  it.each(BLUEPRINT_STAGES)('renders the lifecycle of %s', (name) => {
    let blueprint: UseBlueprintReturn
    function Timeline() {
      blueprint = useBlueprint({ apiUrl: '', autoFetchApprovals: false, approvalPollInterval: 0 })
      return <StageTimeline stages={blueprint.stages} isRunning={blueprint.isRunning} />
    }
    const view = render(<Timeline />)
    const stage = () => screen.getByText(STAGE_INFO[name].label).closest('.stage-timeline__stage')!
    const event = (type: string, extra: Partial<LoopEvent> = {}) => act(() => {
      blueprint.processEvent({
        id: 'event', type, kind: 'stage', timestamp: '2026-10-18T10:00:00Z',
        metadata: { stage_name: name }, ...extra,
      } as LoopEvent)
    })

    expect(stage()).toHaveClass('stage-timeline__stage--pending')
    event('stage_start')
    expect(blueprint!.currentStage).toBe(name)
    expect(stage()).toHaveClass('stage-timeline__stage--running')
    event('stage_retry', { metadata: { stage_name: name, attempt: 2 } })
    expect(within(stage() as HTMLElement).getByText('Attempt 2')).toBeInTheDocument()
    event('stage_failed', { error: 'check refused' })
    expect(stage()).toHaveClass('stage-timeline__stage--failed')
    expect(within(stage() as HTMLElement).getByTitle('check refused')).toBeInTheDocument()
    act(() => blueprint.reset())
    expect(stage()).toHaveClass('stage-timeline__stage--pending')
    event('stage_start')
    event('stage_complete', { timestamp: '2026-10-18T10:00:02Z' })
    expect(stage()).toHaveClass('stage-timeline__stage--completed')
    expect(within(stage() as HTMLElement).getByText('2.0s')).toBeInTheDocument()

    // An unknown stage must still be ignored rather than corrupting the list.
    event('stage_start', { metadata: { stage_name: 'unknown_stage' } })
    expect(blueprint!.currentStage).toBeUndefined()
    expect(blueprint!.stages.map((stage) => stage.name)).toEqual(BLUEPRINT_STAGES)
    view.unmount()
    render(<Timeline />)
    expect(stage()).toHaveClass('stage-timeline__stage--pending')
  })
})
