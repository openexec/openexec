import type { Session } from './types'

export function adoptProjectChat(chosen: Session): Promise<Session> {
  return fetch(`/api/projects/${encodeURIComponent(chosen.projectId)}/chat/adopt`, {
    method: 'POST', body: JSON.stringify({ sessionId: chosen.id }),
  }).then(response => response.json())
}
