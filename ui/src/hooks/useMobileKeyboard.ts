import { useState, useEffect, useCallback } from 'react';

interface MobileKeyboardState {
  /** Whether the keyboard is currently visible */
  isOpen: boolean;
  /** Estimated keyboard height in pixels */
  keyboardHeight: number;
}

/**
 * Hook to detect virtual keyboard state on mobile devices.
 * Uses viewport resize detection as a proxy for keyboard visibility.
 */
export function useMobileKeyboard(): MobileKeyboardState {
  const [state, setState] = useState<MobileKeyboardState>({
    isOpen: false,
    keyboardHeight: 0,
  });

  useEffect(() => {
    // Only run on mobile devices
    if (typeof window === 'undefined') return;

    // Store initial viewport height
    const initialHeight = window.innerHeight;
    let lastHeight = initialHeight;

    const handleResize = () => {
      const currentHeight = window.innerHeight;
      const heightDiff = lastHeight - currentHeight;

      // Keyboard likely opened (viewport shrank significantly)
      if (heightDiff > 100) {
        setState({
          isOpen: true,
          keyboardHeight: heightDiff,
        });
      }
      // Keyboard likely closed (viewport grew back)
      else if (currentHeight > lastHeight + 100) {
        setState({
          isOpen: false,
          keyboardHeight: 0,
        });
      }

      lastHeight = currentHeight;
    };

    // Use visualViewport API if available (more accurate)
    if (window.visualViewport) {
      window.visualViewport.addEventListener('resize', handleResize);
      return () => window.visualViewport?.removeEventListener('resize', handleResize);
    }

    // Fallback to window resize
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return state;
}

/**
 * Hook to scroll input into view when keyboard opens.
 * Useful for chat input fields.
 */
export function useScrollIntoViewOnFocus(
  elementRef: React.RefObject<HTMLElement>,
  options?: ScrollIntoViewOptions
) {
  const handleFocus = useCallback(() => {
    // Small delay to let keyboard animation start
    setTimeout(() => {
      elementRef.current?.scrollIntoView({
        behavior: 'smooth',
        block: 'center',
        ...options,
      });
    }, 100);
  }, [elementRef, options]);

  useEffect(() => {
    const element = elementRef.current;
    if (!element) return;

    element.addEventListener('focus', handleFocus);
    return () => element.removeEventListener('focus', handleFocus);
  }, [elementRef, handleFocus]);
}

/**
 * Hook to adjust layout when keyboard is open.
 * Returns CSS properties to apply to the container.
 */
export function useKeyboardAvoidingStyles(): React.CSSProperties {
  const { isOpen, keyboardHeight } = useMobileKeyboard();

  if (!isOpen) {
    return {};
  }

  return {
    // Adjust padding to account for keyboard
    paddingBottom: `${keyboardHeight}px`,
    // Optional: transition for smooth adjustment
    transition: 'padding-bottom 0.2s ease-out',
  };
}

export default useMobileKeyboard;
