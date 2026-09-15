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
          <stop stopColor="#141326" />
          <stop offset="1" stopColor="#38203e" />
        </linearGradient>
        <linearGradient id="night-sun" x2="0" y2="1">
          <stop stopColor="#fc9b92" />
          <stop offset=".53" stopColor="#e45884" />
          <stop offset="1" stopColor="#6e335f" />
        </linearGradient>
        <linearGradient id="night-fade">
          <stop stopColor="#100f19" />
          <stop offset=".4" stopColor="#100f19" stopOpacity="0" />
        </linearGradient>
        <pattern
          id="night-rain"
          width="29"
          height="31"
          patternUnits="userSpaceOnUse"
        >
          <path d="M8 3v8m17 13v4" stroke="#b0a1ca" strokeOpacity=".08" />
        </pattern>
      </defs>
      <path fill="url(#night-sky)" d="M0 0h480v220H0z" />
      <g fill="#705b8d" opacity=".25">
        <path d="M56 46h51v2H56zM73 43h48v3H73zM249 26h73v3h-73zM277 23h29v3h-29zM325 71h93v3h-93zM389 68h47v3h-47zM151 15h34v2h-34z" />
      </g>
      <path
        d="M334 21h38v3h10v4h8v7h5v9h4v36h-4v9h-5v7h-8v4h-10v3h-38v-3h-10v-4h-8v-7h-5v-9h-4V44h4v-9h5v-7h8v-4h10z"
        fill="url(#night-sun)"
      />
      <path
        d="M307 66h93v3h-93zm0 11h93v4h-93zm5 11h85v5h-85zm10 11h64v5h-64z"
        fill="#2a1c37"
      />
      <g fill="#29223b">
        <path d="M0 155h22v-40h31v15h22v-53h17v-8h7v60h25v-31h15v36h18v-12h29v-45h12v-5h10v62h37v-30h27v20h27v-21h32v30h29v-21h27v18h31V97h21v36h27v-22h34v106H0z" />
      </g>
      {buildings.map(([x, y, w, h], index) => (
        <g key={x}>
          <path
            d={`M${x} ${y}h${w}v${h}h-${w}z`}
            fill={index % 3 === 0 ? "#161624" : "#1d1b2d"}
          />
          <path d={`M${x} ${y}h${w}v2h-${w}z`} fill="#4c3456" />
          <path d={`M${x + 5} ${y - 5}h${w - 12}v5H${x + 5}z`} fill="#222032" />
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
                    fill={(row + col + index) % 3 === 0 ? "#c86e87" : "#477278"}
                    opacity={(row + index) % 3 === 0 ? 0.65 : 0.28}
                  />
                ),
            ),
          )}
        </g>
      ))}
      <path
        d="M302 49V30h2v19M298 38h10v1h-10M457 57V33h1v24M125 69V49h2v20"
        fill="#635470"
      />
      <path d="M288 52h37v2h-37" fill="#f0809f" opacity=".7" />
      <path d="M285 55h1v94h-1M328 52h1v112h-1" fill="#5b3256" />
      <path
        d="M0 174h79v-9h12v-12h11v-6h42v3h9v11h12v24h37v-9h18v12h29v-22h18v-5h32v8h26v20h45v-12h62v-8h15v-5h44v9h12v46H0z"
        fill="#0f111c"
      />
      <path
        d="M79 165h12m11-18h42m79 40h22m53-26h26m101 16h61"
        stroke="#825277"
      />
      <path
        d="M51 177v-35h3v35m-9-35h31v2H45m14 2h1v18m1-17h2v17"
        fill="#15121f"
      />
      <path d="M50 139h23v3H50z" fill="#74aba6" opacity=".8" />
      <path
        d="M51 148h-5v2h-4v2h-7v2H22v1H0m52-8h5v2h5v2h9v2h12v1h20v-1h14v-1h9v-2h9v-2h6m-2-1h20v2h24v3h30v2h28v-1h20v-3h20v-2h12m0 0h20v3h24v2h36v-1h28v-3h14"
        stroke="#0e101b"
        strokeWidth="1.5"
      />
      <path d="M319 172v-37h3v37m-9-38h27v3h-27m26 0v3h-5" fill="#141220" />
      <path d="M332 137h9v3h-9" fill="#d78b96" />
      <path
        d="M108 192h268v3H108zM120 200h215v1H120zM79 211h334v2H79z"
        fill="#332237"
      />
      <path
        d="M189 193h23v1h-23m-7 6h40v1h-40m-5 7h22v1h-22m138-13h13v1h-13m-4 7h21v1h-21m61-11h21v1h-21"
        stroke="#a05476"
        opacity=".45"
      />
      <path d="M0 0h480v220H0z" fill="url(#night-rain)" />
      <path d="M0 0h480v220H0z" fill="url(#night-fade)" />
    </svg>
  );
}
