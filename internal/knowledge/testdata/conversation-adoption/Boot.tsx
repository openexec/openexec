import { App } from './App'

// A genuine depth-2 incoming reference; unlike App's other callees this must
// be an affected caller when the hook changes.
export function Boot(props: Parameters<typeof App>[0]) { return <App {...props} /> }
