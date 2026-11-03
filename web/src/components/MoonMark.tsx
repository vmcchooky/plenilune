/**
 * Plenilune brand mark: a single filled crescent.
 *
 * Geometry is intentionally simple so the silhouette stays readable at 16px:
 * one long outer arc and one concave inner arc, no ring, no stroke, no accent.
 * The mark inherits `currentColor`; size is set by the caller.
 */
export function MoonMark({
  size = 20,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="currentColor"
      focusable="false"
      height={size}
      viewBox="0 0 24 24"
      width={size}
    >
      <path d="M12.4 3.2a6.4 6.4 0 0 0 8.4 10.4 8.9 8.9 0 1 1-8.4-10.4Z" />
    </svg>
  );
}
