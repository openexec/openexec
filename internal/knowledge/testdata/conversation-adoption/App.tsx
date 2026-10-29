import { useMemo } from './react'
import { useConversations } from './useConversations'
import { adoptProjectChat } from './adoptProjectChat'
import { unrelatedSibling } from './unrelated'
import { Conversation } from './Conversation'
import type { Session } from './types'

export function App({ current, projectId, openConversation }: {
  current: Session, projectId: string, openConversation: (id: string) => void,
}) {
  const { sessions, upsert: upsertSession, archive: archiveSession, remove: removeSession } = useConversations(true)
  const ownerSessions = useMemo(() => sessions.filter(item => item.projectId === projectId), [sessions, projectId])
  const earlierConversations = useMemo(() => ownerSessions.filter(item =>
    !item.archivedAt && !item.kind && Date.parse(item.createdAt) < Date.parse(current.createdAt)), [ownerSessions, current])

  function closeConversation(deleted: boolean) {
    if (deleted) {
      removeSession(current.id)
      const existing = ownerSessions.find(item => item.id !== current.id && !item.archivedAt)
      if (existing) { openConversation(existing.id); return }
    } else archiveSession(current.id)
    openConversation('')
  }
  async function adoptConversation(chosen: Session) {
    const adopted = await adoptProjectChat(chosen)
    upsertSession(adopted)
    if (current.id !== adopted.id) archiveSession(current.id)
    openConversation(adopted.id)
  }
  return <><Conversation earlier={earlierConversations} onAdoptConversation={adoptConversation} onCloseSession={closeConversation} />
    <button onClick={() => { unrelatedSibling() }} /></>
}
