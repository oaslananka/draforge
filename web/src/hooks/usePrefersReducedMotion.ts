import { useEffect, useState } from 'react';

const reducedMotionQuery = '(prefers-reduced-motion: reduce)';

function currentPreference() {
  return typeof window !== 'undefined' && window.matchMedia(reducedMotionQuery).matches;
}

export default function usePrefersReducedMotion() {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(currentPreference);

  useEffect(() => {
    const mediaQuery = window.matchMedia(reducedMotionQuery);
    const updatePreference = (event: MediaQueryListEvent) => {
      setPrefersReducedMotion(event.matches);
    };

    setPrefersReducedMotion(mediaQuery.matches);
    mediaQuery.addEventListener('change', updatePreference);
    return () => mediaQuery.removeEventListener('change', updatePreference);
  }, []);

  return prefersReducedMotion;
}
