import type { Session } from './types'
import { ConversationList } from './ConversationList'

export function Conversation({ earlier, onAdoptConversation, onCloseSession }: {
  earlier: Session[], onAdoptConversation: (item: Session) => Promise<void>,
  onCloseSession: (deleted: boolean) => void,
}) {
  return <><ConversationList sessions={earlier.slice(0, 5)}
    onSelect={async item => { await onAdoptConversation(item) }} />
    <button onClick={() => onCloseSession(true)} />
    <button onClick={() => onCloseSession(false)} /></>
}
