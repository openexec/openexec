/**
 * Service Worker Registration for OpenExec PWA
 *
 * This module handles service worker lifecycle:
 * - Registration on app start
 * - Update detection
 * - User prompts for updates
 */

export interface ServiceWorkerCallbacks {
  /** Called when a new service worker is ready to take over */
  onUpdate?: (registration: ServiceWorkerRegistration) => void;
  /** Called when service worker is successfully registered for the first time */
  onSuccess?: (registration: ServiceWorkerRegistration) => void;
  /** Called when service worker registration fails */
  onError?: (error: Error) => void;
  /** Called when app is ready to work offline */
  onOfflineReady?: () => void;
}

/**
 * Register the service worker.
 * Should be called once when the app starts.
 */
export async function registerServiceWorker(
  swUrl = '/sw.js',
  callbacks: ServiceWorkerCallbacks = {}
): Promise<ServiceWorkerRegistration | undefined> {
  const { onUpdate, onSuccess, onError, onOfflineReady } = callbacks;

  // Check if service workers are supported
  if (!('serviceWorker' in navigator)) {
    console.log('[SW] Service workers not supported');
    return;
  }

  // Don't register in development (unless explicitly enabled)
  if (import.meta.env.DEV && !import.meta.env.VITE_ENABLE_SW) {
    console.log('[SW] Skipping registration in development');
    return;
  }

  try {
    const registration = await navigator.serviceWorker.register(swUrl, {
      scope: '/',
      type: 'module',
    });

    console.log('[SW] Registered:', registration.scope);

    // Check for updates periodically (every hour)
    setInterval(() => {
      registration.update().catch(console.error);
    }, 60 * 60 * 1000);

    // Handle different registration states
    if (registration.waiting) {
      // There's a waiting service worker
      onUpdate?.(registration);
    }

    registration.onupdatefound = () => {
      const installingWorker = registration.installing;
      if (!installingWorker) return;

      installingWorker.onstatechange = () => {
        if (installingWorker.state === 'installed') {
          if (navigator.serviceWorker.controller) {
            // New update available
            console.log('[SW] Update available');
            onUpdate?.(registration);
          } else {
            // First install - offline ready
            console.log('[SW] Offline ready');
            onOfflineReady?.();
            onSuccess?.(registration);
          }
        }
      };
    };

    return registration;
  } catch (error) {
    console.error('[SW] Registration failed:', error);
    onError?.(error as Error);
    return;
  }
}

/**
 * Unregister all service workers.
 * Useful for troubleshooting or when PWA is disabled.
 */
export async function unregisterServiceWorkers(): Promise<boolean> {
  if (!('serviceWorker' in navigator)) {
    return false;
  }

  try {
    const registrations = await navigator.serviceWorker.getRegistrations();
    const results = await Promise.all(
      registrations.map(registration => registration.unregister())
    );
    return results.every(Boolean);
  } catch (error) {
    console.error('[SW] Unregister failed:', error);
    return false;
  }
}

/**
 * Signal the waiting service worker to take control.
 * Call this after user confirms they want to update.
 */
export function skipWaiting(registration: ServiceWorkerRegistration): void {
  registration.waiting?.postMessage({ type: 'SKIP_WAITING' });
}

/**
 * Check if the app was launched as a PWA (standalone mode).
 */
export function isPWAInstalled(): boolean {
  // Check display-mode media query
  if (window.matchMedia('(display-mode: standalone)').matches) {
    return true;
  }

  // Check iOS Safari
  if ((navigator as any).standalone === true) {
    return true;
  }

  // Check if launched from homescreen (referrer check)
  if (document.referrer.includes('android-app://')) {
    return true;
  }

  return false;
}

/**
 * Get the current service worker state.
 */
export async function getServiceWorkerState(): Promise<{
  isSupported: boolean;
  isRegistered: boolean;
  isUpdateAvailable: boolean;
  isOfflineReady: boolean;
}> {
  if (!('serviceWorker' in navigator)) {
    return {
      isSupported: false,
      isRegistered: false,
      isUpdateAvailable: false,
      isOfflineReady: false,
    };
  }

  const registration = await navigator.serviceWorker.getRegistration();

  return {
    isSupported: true,
    isRegistered: !!registration,
    isUpdateAvailable: !!registration?.waiting,
    isOfflineReady: !!registration?.active,
  };
}

export default registerServiceWorker;
