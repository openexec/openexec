# Chat UI Component Structure

This document describes the component architecture for the OpenExec Chat UI with Session Management.

## Component Hierarchy

```
ChatLayout (main container)
├── SessionSidebar
│   ├── SessionList
│   │   ├── SessionListItem
│   │   └── SessionListItem
│   ├── NewSessionButton
│   └── SessionFilters
│
├── ChatMain
│   ├── ChatHeader
│   │   ├── SessionTitle
│   │   ├── ModelIndicator
│   │   ├── LoopStatusBadge
│   │   └── ChatActions (pause/resume/stop)
│   │
│   ├── MessageList
│   │   ├── MessageGroup
│   │   │   ├── UserMessage
│   │   │   └── AssistantMessage
│   │   │       ├── MessageContent
│   │   │       └── ToolCallList
│   │   │           └── ToolCallCard
│   │   │               ├── ToolCallInput
│   │   │               ├── ToolCallOutput
│   │   │               └── ToolCallApproval
│   │   ├── StreamingMessage
│   │   └── SystemMessage
│   │
│   ├── ChatInput
│   │   ├── InputTextarea
│   │   ├── SendButton
│   │   └── InputToolbar
│   │
│   └── LoopProgressBar
│
├── EventPanel (collapsible)
│   ├── EventFilter
│   └── EventList
│       └── EventItem
│
└── CostPanel (collapsible)
    ├── CostSummary
    ├── TokenUsage
    └── BudgetProgress
```

## Component Descriptions

### Layout Components

#### `ChatLayout`
Main container component that manages the overall layout grid.
- **Props**: None (uses store directly)
- **State**: Panel visibility (sidebar, events, cost)
- **Responsibilities**: Responsive layout, keyboard shortcuts

#### `SessionSidebar`
Left sidebar containing session management.
- **Props**: `onSessionSelect`, `onNewSession`
- **State**: Expanded/collapsed state
- **Features**: Session search, status filter, project grouping

### Session Components

#### `SessionList`
Virtualized list of session items.
- **Props**: `sessions`, `selectedId`, `onSelect`
- **Features**: Virtual scrolling, loading states

#### `SessionListItem`
Individual session preview in the list.
- **Props**: `session`, `isSelected`, `onClick`
- **Features**: Title, model badge, last message preview, timestamp

#### `NewSessionButton`
Button to create a new session.
- **Props**: `onClick`
- **Opens**: Model selection dialog

### Chat Components

#### `ChatHeader`
Header bar showing session info and controls.
- **Props**: `session`, `loopState`
- **Actions**: Pause, Resume, Stop, Settings

#### `SessionTitle`
Editable session title with inline editing.
- **Props**: `title`, `onUpdate`
- **Features**: Double-click to edit, auto-save

#### `ModelIndicator`
Shows current provider/model with status.
- **Props**: `provider`, `model`, `status`
- **Features**: Provider icon, model name, status dot

#### `LoopStatusBadge`
Badge showing current loop state.
- **Props**: `state`, `iteration`
- **States**: Running, Paused, Stopped, Complete, Error

#### `ChatActions`
Action buttons for loop control.
- **Props**: `loopState`, `onPause`, `onResume`, `onStop`
- **Features**: Conditional button visibility

### Message Components

#### `MessageList`
Scrollable container for messages with virtualization.
- **Props**: `messages`, `streamingMessage`
- **Features**: Auto-scroll, load more, scroll-to-bottom

#### `MessageGroup`
Groups consecutive messages from same role.
- **Props**: `messages`, `role`
- **Purpose**: Visual grouping, avatar display

#### `UserMessage`
User-submitted message display.
- **Props**: `message`
- **Features**: Text content, timestamp, copy action

#### `AssistantMessage`
AI assistant response display.
- **Props**: `message`, `toolCalls`
- **Features**: Markdown rendering, code highlighting, tool calls

#### `MessageContent`
Renders message content with proper formatting.
- **Props**: `content`, `role`
- **Features**: Markdown, syntax highlighting, links

#### `StreamingMessage`
Displays in-progress streaming response.
- **Props**: `content`, `isStreaming`
- **Features**: Typing indicator, progressive reveal

#### `SystemMessage`
System/context injection message.
- **Props**: `message`
- **Features**: Collapsed by default, expandable

### Tool Call Components

#### `ToolCallList`
Container for tool calls in a message.
- **Props**: `toolCalls`
- **Layout**: Vertical stack of tool cards

#### `ToolCallCard`
Individual tool call display card.
- **Props**: `toolCall`, `onApprove`, `onReject`
- **Sections**: Header, input, output, status

#### `ToolCallInput`
Collapsible JSON input viewer.
- **Props**: `input`, `toolName`
- **Features**: JSON syntax highlighting, copy

#### `ToolCallOutput`
Tool execution result display.
- **Props**: `output`, `isError`, `duration`
- **Features**: Output formatting, error styling

#### `ToolCallApproval`
Approval/rejection controls for pending tools.
- **Props**: `toolCallId`, `riskLevel`, `onApprove`, `onReject`
- **Features**: Risk badge, reason input

### Input Components

#### `ChatInput`
Main message input area.
- **Props**: `onSubmit`, `disabled`
- **State**: Input content, multiline
- **Features**: Auto-resize, keyboard shortcuts

#### `InputTextarea`
Auto-resizing textarea component.
- **Props**: `value`, `onChange`, `onSubmit`
- **Features**: Ctrl+Enter to send, max height

#### `SendButton`
Submit button with loading state.
- **Props**: `onClick`, `disabled`, `loading`
- **Features**: Loading spinner, tooltip

#### `InputToolbar`
Additional input actions toolbar.
- **Props**: `onAttach`, `onClear`
- **Features**: File attach, clear input

### Event Components

#### `EventPanel`
Collapsible panel showing loop events.
- **Props**: `events`, `filters`
- **Features**: Real-time updates, filtering

#### `EventFilter`
Filter controls for event list.
- **Props**: `filters`, `onChange`
- **Options**: Event type, kind, time range

#### `EventList`
Virtualized list of events.
- **Props**: `events`
- **Features**: Auto-scroll, color coding

#### `EventItem`
Individual event display.
- **Props**: `event`
- **Features**: Type icon, timestamp, expandable details

### Cost Components

#### `CostPanel`
Collapsible panel showing cost info.
- **Props**: `cost`, `budget`
- **Features**: Real-time updates

#### `CostSummary`
Total cost display.
- **Props**: `sessionTotal`, `iterationCost`
- **Format**: USD currency

#### `TokenUsage`
Token count display.
- **Props**: `inputTokens`, `outputTokens`
- **Features**: Input/output breakdown

#### `BudgetProgress`
Budget limit progress bar.
- **Props**: `used`, `limit`
- **Features**: Warning threshold, color coding

## File Structure

```
ui/src/
├── components/
│   └── chat/
│       ├── index.ts                 # Public exports
│       ├── README.md                # This file
│       │
│       ├── layout/
│       │   ├── ChatLayout.tsx
│       │   ├── ChatMain.tsx
│       │   └── index.ts
│       │
│       ├── session/
│       │   ├── SessionSidebar.tsx
│       │   ├── SessionList.tsx
│       │   ├── SessionListItem.tsx
│       │   ├── NewSessionButton.tsx
│       │   ├── SessionFilters.tsx
│       │   └── index.ts
│       │
│       ├── header/
│       │   ├── ChatHeader.tsx
│       │   ├── SessionTitle.tsx
│       │   ├── ModelIndicator.tsx
│       │   ├── LoopStatusBadge.tsx
│       │   ├── ChatActions.tsx
│       │   └── index.ts
│       │
│       ├── messages/
│       │   ├── MessageList.tsx
│       │   ├── MessageGroup.tsx
│       │   ├── UserMessage.tsx
│       │   ├── AssistantMessage.tsx
│       │   ├── MessageContent.tsx
│       │   ├── StreamingMessage.tsx
│       │   ├── SystemMessage.tsx
│       │   └── index.ts
│       │
│       ├── tools/
│       │   ├── ToolCallList.tsx
│       │   ├── ToolCallCard.tsx
│       │   ├── ToolCallInput.tsx
│       │   ├── ToolCallOutput.tsx
│       │   ├── ToolCallApproval.tsx
│       │   └── index.ts
│       │
│       ├── input/
│       │   ├── ChatInput.tsx
│       │   ├── InputTextarea.tsx
│       │   ├── SendButton.tsx
│       │   ├── InputToolbar.tsx
│       │   └── index.ts
│       │
│       ├── events/
│       │   ├── EventPanel.tsx
│       │   ├── EventFilter.tsx
│       │   ├── EventList.tsx
│       │   ├── EventItem.tsx
│       │   └── index.ts
│       │
│       └── cost/
│           ├── CostPanel.tsx
│           ├── CostSummary.tsx
│           ├── TokenUsage.tsx
│           ├── BudgetProgress.tsx
│           └── index.ts
│
├── store/
│   ├── chatStore.ts                 # Zustand store
│   ├── chatSelectors.ts             # Derived state selectors
│   └── index.ts
│
├── hooks/
│   ├── useChat.ts                   # Main chat hook
│   ├── useWebSocket.ts              # WebSocket connection
│   ├── useMessages.ts               # Message operations
│   ├── useSession.ts                # Session operations
│   ├── useToolCalls.ts              # Tool call operations
│   └── index.ts
│
├── types/
│   ├── chat.ts                      # Chat domain types
│   ├── store.ts                     # Store types
│   └── index.ts
│
└── utils/
    ├── messageFormatting.ts         # Message content formatting
    ├── costFormatting.ts            # Cost/token formatting
    ├── eventFormatting.ts           # Event display helpers
    └── index.ts
```

## State Management

The chat UI uses Zustand for state management with the following store structure:

### Store Slices

1. **Connection Slice**: WebSocket connection state
2. **Sessions Slice**: Session list and current session
3. **Messages Slice**: Message history and streaming
4. **ToolCalls Slice**: Tool call state and approvals
5. **Loop Slice**: Agent loop state and config
6. **Events Slice**: Loop events and filters
7. **Cost Slice**: Cost tracking and budget
8. **Providers Slice**: Provider/model information
9. **Input Slice**: Chat input state

### Persistence

- Session list cached in localStorage
- Current session persisted on navigation
- Event filters persisted per-session

## WebSocket Protocol

### Connection Flow

1. Connect to `ws://{host}/api/chat/ws`
2. Authenticate with session token
3. Subscribe to session events
4. Receive real-time updates

### Message Types

**Client → Server:**
- `send_message`: Submit user message
- `approve_tool`: Approve tool call
- `reject_tool`: Reject tool call
- `pause`: Pause loop
- `resume`: Resume loop
- `stop`: Stop loop

**Server → Client:**
- `message`: Complete message
- `streaming_chunk`: Streaming content
- `tool_call_update`: Tool call status change
- `event`: Loop event
- `error`: Error notification

## Accessibility

- Full keyboard navigation
- ARIA labels on interactive elements
- Focus management on message send
- Screen reader announcements for events
- High contrast support for status indicators

## Performance Considerations

- Virtual scrolling for message/event lists
- Debounced input for search/filters
- Memoized selectors for computed state
- Optimistic updates for tool approvals
- Event batching for high-frequency updates
