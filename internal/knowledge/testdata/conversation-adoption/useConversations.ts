import { useCallback, useState } from './react'
import type { Session } from './types'

export function useConversations(active: boolean) {
  const [sessions, setSessions] = useState<Session[]>([])
  const apply = useCallback((next: Session[] | null | undefined) => {
    if (Array.isArray(next)) setSessions(next)
  }, [])
  const refresh = useCallback(async () => {
    if (active) apply(await fetch('/api/sessions?includeArchived=true').then(response => response.json()))
  }, [active, apply])
  const upsert = useCallback((session: Session) => setSessions(items =>
    items.some(item => item.id === session.id)
      ? items.map(item => item.id === session.id ? session : item) : [session, ...items]), [])
  const archive = useCallback((id: string) => setSessions(items =>
    items.map(item => item.id === id ? { ...item, archivedAt: 'now' } : item)), [])
  const remove = useCallback((id: string) => {
    setSessions(items => items.filter(item => item.id !== id))
  }, [])
  return { sessions, refresh, apply, upsert, archive, remove }
}
