// Compiler context for the synthetic React-shaped fixture, not an extractor mock.
export declare function useState<T>(initial: T): [T, (next: T | ((previous: T) => T)) => void]
export declare function useMemo<T>(create: () => T, dependencies: unknown[]): T
export declare function useCallback<T>(callback: T, dependencies: unknown[]): T
declare global {
  namespace JSX { interface IntrinsicElements { button: { onClick: () => void } } }
}
