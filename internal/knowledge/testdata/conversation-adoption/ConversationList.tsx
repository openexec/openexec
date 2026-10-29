import type { Session } from './types'

export function ConversationList({ sessions, onSelect }: {
  sessions: Session[], onSelect: (item: Session) => Promise<void>,
}) {
  return sessions.map(item => <button onClick={() => { void onSelect(item) }} />)
}
