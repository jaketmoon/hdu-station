// Original vector pixel scene: bundled, offline, with no game assets or remote images.
export function NightScene() {
  const buildings = [
    [8, 121, 34, 96],
    [46, 93, 27, 124],
    [77, 133, 38, 84],
    [120, 69, 33, 148],
    [157, 102, 42, 115],
    [205, 123, 37, 94],
    [247, 81, 34, 136],
    [285, 49, 43, 168],
    [333, 108, 29, 109],
    [368, 81, 38, 136],
    [413, 110, 30, 107],
    [448, 57, 36, 160],
  ];
  return (
    <svg
      className="night-scene"
      viewBox="0 0 480 220"
      fill="none"
      aria-hidden="true"
      shapeRendering="crispEdges"
    >
      <defs>
        <linearGradient id="night-sky" x2="0" y2="1">
          <stop stopColor="var(--scene-sky-top)" />
          <stop offset="1" stopColor="var(--scene-sky-bottom)" />
        </linearGradient>
        <linearGradient id="night-fade">
          <stop stopColor="var(--panel)" />
          <stop offset=".4" stopColor="var(--panel)" stopOpacity="0" />
        </linearGradient>
        <pattern
          id="night-rain"
          width="29"
          height="31"
          patternUnits="userSpaceOnUse"
        >
          <path
            d="M8 3v8m17 13v4"
            stroke="var(--scene-rain)"
            strokeOpacity=".08"
          />
        </pattern>
      </defs>
      <path fill="url(#night-sky)" d="M0 0h480v220H0z" />
      <g fill="var(--scene-haze)" opacity=".25">
        <path d="M56 46h51v2H56zM73 43h48v3H73zM249 26h73v3h-73zM277 23h29v3h-29zM325 71h93v3h-93zM389 68h47v3h-47zM151 15h34v2h-34z" />
      </g>
      <g stroke="var(--scene-orbit)" opacity=".55">
        <path d="M332 17h38l25 25v38l-25 25h-38l-25-25V42z" />
        <path d="M337 27h28l20 20v28l-20 20h-28l-20-20V47z" opacity=".4" />
        <path d="M350 8v16m0 73v15m-52-51h17m69 0h19" />
        <path
          d="M316 33l9 9m53 39 9 9M316 89l9-9m53-39 9-9"
          stroke="var(--scene-orbit-marker)"
        />
      </g>
      <path
        d="M347 46h7v7h-7zM343 58h15v3h-15zM347 65h7v12h-7z"
        fill="var(--scene-orbit-core)"
        opacity=".35"
      />
      <g fill="var(--scene-skyline)">
        <path d="M0 155h22v-40h31v15h22v-53h17v-8h7v60h25v-31h15v36h18v-12h29v-45h12v-5h10v62h37v-30h27v20h27v-21h32v30h29v-21h27v18h31V97h21v36h27v-22h34v106H0z" />
      </g>
      {buildings.map(([x, y, w, h], index) => (
        <g key={x}>
          <path
            d={`M${x} ${y}h${w}v${h}h-${w}z`}
            fill={
              index % 3 === 0
                ? "var(--scene-building-dark)"
                : "var(--scene-building-light)"
            }
          />
          <path d={`M${x} ${y}h${w}v2h-${w}z`} fill="var(--scene-roof-edge)" />
          <path
            d={`M${x + 5} ${y - 5}h${w - 12}v5H${x + 5}z`}
            fill="var(--scene-roof)"
          />
          {Array.from({ length: Math.floor(h / 11) - 1 }, (_, row) =>
            Array.from(
              { length: Math.floor(w / 8) - 1 },
              (_, col) =>
                (row * 3 + col * 7 + index) % 5 < 2 && (
                  <rect
                    key={`${row}-${col}`}
                    x={x + 5 + col * 8}
                    y={y + 8 + row * 11}
                    width={3}
                    height={4}
                    fill={
                      (row + col + index) % 3 === 0
                        ? "var(--scene-window-warm)"
                        : "var(--scene-window-cool)"
                    }
                    opacity={(row + index) % 3 === 0 ? 0.65 : 0.28}
                  />
                ),
            ),
          )}
        </g>
      ))}
      <path
        d="M302 49V30h2v19M298 38h10v1h-10M457 57V33h1v24M125 69V49h2v20"
        fill="var(--scene-antenna)"
      />
      <path d="M288 52h37v2h-37" fill="var(--scene-roof-light)" opacity=".7" />
      <path
        d="M285 55h1v94h-1M328 52h1v112h-1"
        fill="var(--scene-building-edge)"
      />
      <path
        d="M0 174h79v-9h12v-12h11v-6h42v3h9v11h12v24h37v-9h18v12h29v-22h18v-5h32v8h26v20h45v-12h62v-8h15v-5h44v9h12v46H0z"
        fill="var(--scene-foreground)"
      />
      <path
        d="M79 165h12m11-18h42m79 40h22m53-26h26m101 16h61"
        stroke="var(--scene-foreground-edge)"
      />
      <path
        d="M51 177v-35h3v35m-9-35h31v2H45m14 2h1v18m1-17h2v17"
        fill="var(--scene-pole)"
      />
      <path d="M50 139h23v3H50z" fill="var(--scene-lamp-cool)" opacity=".8" />
      <path
        d="M51 148h-5v2h-4v2h-7v2H22v1H0m52-8h5v2h5v2h9v2h12v1h20v-1h14v-1h9v-2h9v-2h6m-2-1h20v2h24v3h30v2h28v-1h20v-3h20v-2h12m0 0h20v3h24v2h36v-1h28v-3h14"
        stroke="var(--scene-cable)"
        strokeWidth="1.5"
      />
      <path
        d="M319 172v-37h3v37m-9-38h27v3h-27m26 0v3h-5"
        fill="var(--scene-street-fixture)"
      />
      <path d="M332 137h9v3h-9" fill="var(--scene-lamp-warm)" />
      <path
        d="M108 192h268v3H108zM120 200h215v1H120zM79 211h334v2H79z"
        fill="var(--scene-road)"
      />
      <path
        d="M189 193h23v1h-23m-7 6h40v1h-40m-5 7h22v1h-22m138-13h13v1h-13m-4 7h21v1h-21m61-11h21v1h-21"
        stroke="var(--scene-reflection)"
        opacity=".45"
      />
      <g fill="var(--scene-signal)">
        <path
          d="M252 96h3v28h-3zM252 130h3v8h-3zM374 94h24v3h-24zM374 100h16v2h-16z"
          opacity=".7"
        />
        <path d="M290 62h3v3h-3zM298 62h3v3h-3zM306 62h3v3h-3z" />
        <path
          d="M232 204h37v1h-37zM242 208h19v1h-19zM355 197h49v1h-49z"
          opacity=".4"
        />
      </g>
      <path d="M0 0h480v220H0z" fill="url(#night-rain)" />
      <path d="M0 0h480v220H0z" fill="url(#night-fade)" />
    </svg>
  );
}
