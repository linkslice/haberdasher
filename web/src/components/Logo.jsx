export function HaberdasherLogo({ size = 32 }) {
  const s = size
  const scale = s / 160
  return (
    <svg width={s} height={s} viewBox="0 0 160 160" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="42" y="110" width="76" height="16" rx="4" fill="#185FA5"/>
      <rect x="22" y="118" width="116" height="14" rx="3" fill="#0C447C"/>
      <rect x="52" y="32" width="56" height="80" rx="8" fill="#185FA5"/>
      <rect x="52" y="32" width="56" height="22" rx="8" fill="#0C447C"/>
      <rect x="52" y="48" width="56" height="6" fill="#0C447C"/>
      {/* dots representing proxy routing */}
      <circle cx="72" cy="76" r="6" fill="none" stroke="#85B7EB" strokeWidth="2"/>
      <circle cx="95" cy="76" r="6" fill="none" stroke="#85B7EB" strokeWidth="2"/>
      <line x1="78" y1="76" x2="89" y2="76" stroke="#85B7EB" strokeWidth="1.5" markerEnd="url(#ah)"/>
      {/* hat band highlight */}
      <rect x="58" y="36" width="32" height="10" rx="2" fill="#378ADD" opacity="0.6"/>
      <defs>
        <marker id="ah" viewBox="0 0 8 8" refX="6" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
          <path d="M1 1L6 4L1 7" fill="none" stroke="#85B7EB" strokeWidth="1.5" strokeLinecap="round"/>
        </marker>
      </defs>
    </svg>
  )
}
