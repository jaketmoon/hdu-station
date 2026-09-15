import { PastoralBackdrop, PastoralPlantings } from "./PastoralLandscape";

// HDU's circular tower and entrance truss in a city or pastoral landscape.
// All geometry is bundled locally; materials and lights follow the active theme.
export function CampusGateScene({ pastoral = false }: { pastoral?: boolean }) {
  const towers = [
    [88, 127, 34, 90],
    [132, 94, 36, 123],
    [178, 114, 32, 103],
    [221, 55, 43, 162],
    [277, 97, 39, 120],
    [329, 72, 46, 145],
    [386, 112, 30, 105],
    [430, 85, 37, 132],
  ];
  const trees = [
    [12, 186, 1.25],
    [65, 196, 0.85],
    [179, 203, 0.55],
    [215, 204, 0.55],
    [331, 203, 0.65],
    [371, 201, 0.65],
    [579, 198, 0.8],
    [609, 191, 0.95],
  ];

  return (
    <svg
      className="campus-gate-scene"
      data-landscape={pastoral ? "pastoral" : "city"}
      viewBox="0 0 600 250"
      preserveAspectRatio="xMaxYMax meet"
      fill="none"
      aria-hidden="true"
      shapeRendering="crispEdges"
    >
      <defs>
        <linearGradient id="campus-sky" x2="0" y2="1">
          <stop stopColor="var(--panel)" />
          <stop offset=".3" stopColor="var(--scene-sky-top)" />
          <stop offset="1" stopColor="var(--scene-sky-bottom)" />
        </linearGradient>
        <linearGradient id="campus-fade">
          <stop stopColor="var(--panel)" />
          <stop offset=".2" stopColor="var(--panel)" stopOpacity=".85" />
          <stop offset=".48" stopColor="var(--panel)" stopOpacity="0" />
        </linearGradient>
        <pattern
          id="campus-panels"
          width={pastoral ? 12 : 28}
          height={pastoral ? 8 : 32}
          patternUnits="userSpaceOnUse"
        >
          <path
            d={
              pastoral
                ? "M0 0h12M0 4h12M6 0v4M0 4v4"
                : "M0 0h28M0 1v23l8 8M27 0v10"
            }
            stroke="var(--gate-shell-edge)"
            strokeOpacity=".3"
          />
        </pattern>
        <path
          id="campus-tower-shape"
          fillRule="evenodd"
          d="M477 18h54v39h38v157h-92z M496 31h12v3h5v5h3v12h-3v5h-5v3h-12v-3h-5v-5h-3V39h3v-5h5z"
        />
        <clipPath id="campus-frame">
          <path d="M0 0h600v250H0z" />
        </clipPath>
      </defs>
      <g clipPath="url(#campus-frame)">
        <path fill="url(#campus-sky)" d="M0 0h600v250H0z" />
        {pastoral ? (
          <PastoralBackdrop />
        ) : (
          <>
            <g fill="var(--scene-haze)" opacity=".2">
              <path d="M116 66h103v3H116zM142 62h49v4h-49zM268 36h100v3H268zM294 32h43v4h-43zM335 88h111v3H335zM366 84h55v4h-55z" />
            </g>
            <g stroke="var(--scene-signal)" strokeOpacity=".22">
              <path d="M420 26h26m-13-13v26M270 94h17m-8-8v17M579 76h15m-7-7v14" />
            </g>

            {/* The city sits behind the recognizable foreground gate. */}
            <g stroke="var(--scene-signal)" opacity=".24">
              <path d="M342 25h35l22 22v35l-22 22h-35l-22-22V47z" />
              <path
                d="M347 35h25l17 17v25l-17 17h-25l-17-17V52z"
                opacity=".45"
              />
              <path d="M359 16v17m-48 31h16m63 0h18" />
              <path d="M326 38l9 9m49-9-8 8" stroke="var(--gate-beacon)" />
            </g>
            <path
              d="M57 217V146h31v-23h18v44h21V99h22v-8h13v66h33v-24h22V84h18v-7h14v62h38v-26h26V80h19v-8h14v68h28v-18h19v22h24v-43h24V67h20v94h31v-38h20v94z"
              fill="var(--scene-building-edge)"
              opacity=".16"
            />
            {towers.map(([x, y, width, height], index) => (
              <g className="campus-city-tower" key={x}>
                <path
                  d={`M${x} ${y}h${width}v${height}H${x}z`}
                  fill={
                    index % 3 === 0
                      ? "var(--scene-building-dark)"
                      : "var(--scene-building-light)"
                  }
                />
                <path
                  d={`M${x + width - 8} ${y}h8v${height}h-8z`}
                  fill="var(--scene-building-dark)"
                />
                <path
                  d={`M${x - 2} ${y}h${width + 4}v3H${x - 2}z`}
                  fill="var(--scene-roof-edge)"
                />
                <path
                  d={`M${x + 6} ${y - 6}h${width - 16}v6H${x + 6}z`}
                  fill="var(--scene-building-edge)"
                  opacity=".45"
                />
                <path
                  d={`M${x + width - 8} ${y + 4}v${height - 8}`}
                  stroke="var(--scene-building-edge)"
                  opacity=".4"
                />
                {Array.from(
                  { length: Math.floor(height / 12) - 1 },
                  (_, row) => (
                    <g key={row}>
                      {Array.from(
                        { length: Math.floor((width - 8) / 8) },
                        (_, col) =>
                          (row * 3 + col * 7 + index) % 5 < 2 && (
                            <rect
                              key={col}
                              x={x + 4 + col * 8}
                              y={y + 9 + row * 12}
                              width="3"
                              height="4"
                              fill={
                                (row + col + index) % 5 === 0
                                  ? "var(--scene-window-warm)"
                                  : "var(--scene-window-cool)"
                              }
                              opacity={(row + col) % 3 === 0 ? ".7" : ".35"}
                            />
                          ),
                      )}
                    </g>
                  ),
                )}
                {index % 2 === 1 && (
                  <path
                    d={`M${x + 7} ${y - 6}v-15m-4 6h9`}
                    stroke="var(--scene-roof-edge)"
                  />
                )}
                {index === 3 && (
                  <>
                    <path
                      d={`M${x + 7} ${y + 8}h3v34h-3zM${x + 7} ${y + 47}h3v6h-3z`}
                      fill="var(--scene-signal)"
                      opacity=".7"
                    />
                    <path
                      d={`M${x + 18} ${y + 4}h15v2h-15z`}
                      fill="var(--gate-beacon)"
                      opacity=".8"
                    />
                  </>
                )}
                {index === 5 && (
                  <path
                    d={`M${x + 5} ${y + 13}h24v3h-24zM${x + 5} ${y + 19}h16v2h-16z`}
                    fill="var(--scene-signal)"
                    opacity=".65"
                  />
                )}
              </g>
            ))}
          </>
        )}
        {/* Armored tower: an illuminated aperture, inset panels, and conduits. */}
        <g className="campus-gate-tower">
          <path
            d="M569 60h8v154h-8zM531 18h5v39h-5z"
            fill="var(--gate-shell-shadow)"
          />
          <use href="#campus-tower-shape" fill="var(--gate-shell)" />
          <use href="#campus-tower-shape" fill="url(#campus-panels)" />
          <path
            d="M477 18h54v2h-54zM477 20h2v191h-2zM531 57h38v2h-38z"
            fill="var(--gate-shell-edge)"
          />
          {!pastoral && (
            <>
              <path
                d="M495 27h14v3h7v7h3v16h-3v7h-7v3h-14v-3h-7v-7h-3V37h3v-7h7z M496 31h12v3h5v5h3v12h-3v5h-5v3h-12v-3h-5v-5h-3V39h3v-5h5z"
                fillRule="evenodd"
                fill="var(--scene-signal)"
                opacity=".5"
              />
              <path
                d="M495 28h14m8 10v14m-22 10h14m-23-24v14"
                stroke="var(--scene-signal)"
                strokeWidth="2"
              />
              <path
                d="M480 21h16m-16 2v45m48-44v22m-51 81v20"
                stroke="var(--gate-glass-frame)"
                opacity=".7"
              />
            </>
          )}
          <path d="M493 76h15v29h-15z" fill="var(--scene-sky-bottom)" />
          <path d="M508 72h9v141h-9z" fill="var(--gate-shell-shadow)" />
          <path
            d="M493 77h3v16h-3zM483 159h6v43h-6z"
            fill="var(--gate-shell-shadow)"
          />
          {!pastoral && (
            <path
              d="M484 162h2v24h-2zM484 190h2v8h-2z"
              fill="var(--scene-signal)"
              opacity=".65"
            />
          )}

          {/* The glazed stairwell becomes a vertical light well. */}
          <path d="M496 97h24v113h-24z" fill="var(--scene-building-dark)" />
          <path d="M498 99h18v108h-18z" fill="var(--gate-glass)" />
          <path d="M516 98h4v113h-4z" fill="var(--gate-shell-shadow)" />
          {!pastoral && (
            <>
              <path
                d="M501 100h4v106h-4z"
                fill="var(--scene-signal)"
                opacity=".15"
              />
              <path
                d="M502 100v106"
                stroke="var(--scene-signal)"
                opacity=".85"
              />
            </>
          )}
          <path
            d="M497 99v109m8-109v109m10-109v109"
            stroke="var(--gate-glass-frame)"
          />
          {Array.from({ length: 5 }, (_, floor) => (
            <g key={floor}>
              <path
                d={`M493 ${96 + floor * 23}h29v3h-29z`}
                fill="var(--gate-glass-frame)"
              />
              <path
                d={`M498 ${105 + floor * 23}h18m-18 6h18m-18 6h18`}
                stroke="var(--gate-glass-frame)"
                strokeOpacity=".55"
              />
            </g>
          ))}
          {pastoral ? (
            <g>
              <path d="M537 72h24v44h-24z" fill="var(--gate-truss)" />
              <path d="M540 75h18v37h-18z" fill="var(--gate-glass)" />
              <path
                d="M549 75v37m-9-25h18m-18 12h18"
                stroke="var(--gate-glass-frame)"
                strokeWidth="2"
              />
              <path d="M534 115h30v4h-30z" fill="var(--gate-truss-edge)" />
            </g>
          ) : (
            <>
              <path
                d="M537 70h24v51h-24zM536 164h25v16h-25zM536 186h25v16h-25z"
                fill="var(--gate-shell-shadow)"
              />
              <path
                d="M539 73h20v1h-20zM539 78h3v20h-3zM539 101h3v5h-3zM548 79h10v2h-10zM548 85h7v2h-7zM548 91h10v2h-10zM539 114h19v2h-19z"
                fill="var(--scene-signal)"
                opacity=".75"
              />
              <path
                d="M539 168h18m-18 4h18m-18 4h18M539 190h18m-18 4h18m-18 4h18"
                stroke="var(--gate-shell-edge)"
                opacity=".7"
              />
              <path
                d="M551 104h3v3h-3zM556 104h3v3h-3zM535 61h16v2h-16zM549 207h14v2h-14z"
                fill="var(--gate-beacon)"
              />
            </>
          )}
          <path
            d="M561 91h2v3h-2zM561 112h2v3h-2zM561 133h2v3h-2zM561 154h2v3h-2zM561 175h2v3h-2z"
            fill="var(--gate-shell-shadow)"
          />
        </g>

        {/* The original pier groups carry armored collars and narrow guide lights. */}
        {[106, 408].map((x, index) => (
          <g key={x}>
            <path
              d={`M${x - 5} ${166 - index * 16}h53v7h-53z`}
              fill="var(--gate-pier-light)"
            />
            {Array.from({ length: 4 }, (_, column) => (
              <g key={column}>
                <rect
                  x={x + column * 12}
                  y={172 - index * 16}
                  width="9"
                  height={48 + index * 16}
                  fill="var(--gate-pier)"
                />
                <path
                  d={`M${x + column * 12} ${172 - index * 16}h2v${48 + index * 16}h-2z`}
                  fill="var(--gate-pier-light)"
                />
                <path
                  d={`M${x + 7 + column * 12} ${172 - index * 16}h2v${48 + index * 16}h-2z`}
                  fill="var(--gate-pier-shadow)"
                />
                <path
                  d={`M${x + column * 12} 192h9m-9 15h9`}
                  stroke="var(--gate-pier-shadow)"
                  strokeOpacity=".5"
                />
                {!pastoral && column % 2 === 0 && (
                  <path
                    d={`M${x + 3 + column * 12} ${178 - index * 16}v${38 + index * 16}`}
                    stroke="var(--scene-signal)"
                    opacity=".65"
                  />
                )}
              </g>
            ))}
            <path
              d={`M${x - 3} 220h51v3h-51z`}
              fill="var(--gate-pier-shadow)"
            />
          </g>
        ))}

        {/* Two offset lattice faces preserve the reference's triangular steelwork. */}
        <g className="campus-gate-truss" transform="matrix(1 -.055 0 1 0 0)">
          <path
            d="M0 150h600v25H0z"
            fill="var(--gate-truss-shadow)"
            fillOpacity=".17"
          />
          <g stroke="var(--gate-truss-shadow)" strokeWidth="1.5" opacity=".6">
            {Array.from({ length: 20 }, (_, index) => (
              <path
                key={index}
                d={`M${index * 30} 151l15 24 15-24m-30 24 15-24 15 24`}
              />
            ))}
            <path d="M0 151h600M0 175h600" strokeWidth="2" />
          </g>
          <g stroke="var(--gate-truss)" strokeWidth="1.5">
            {Array.from({ length: 20 }, (_, index) => (
              <path
                key={index}
                d={`M${index * 30} 158l15 24 15-24m-30 24 15-24 15 24m-30-24v24`}
              />
            ))}
            <path d="M0 157h600M0 183h600" strokeWidth="3" />
            <path d="M0 170h600" strokeWidth="1" />
          </g>
          <g stroke="var(--gate-truss-edge)">
            <path d="M0 155h600M0 180h600" />
            {Array.from({ length: 20 }, (_, index) => (
              <path
                key={index}
                d={`M${index * 30} 158l15 22m-15-29v6m15 18v6`}
                strokeOpacity=".65"
              />
            ))}
          </g>
          <path d="M0 184h600v8H0z" fill="var(--gate-truss-shadow)" />
          {!pastoral && (
            <>
              <path
                d="M0 185h600v1H0z"
                fill="var(--scene-signal)"
                opacity=".65"
              />
              {[108, 228, 348, 468, 588].map((x) => (
                <g key={x}>
                  <path
                    d={`M${x - 8} 177h19v15h-19z`}
                    fill="var(--gate-shell-shadow)"
                  />
                  <path
                    d={`M${x - 6} 178h15v2h-15z`}
                    fill="var(--gate-truss-edge)"
                  />
                  <path
                    d={`M${x - 4} 184h3v3h-3zM${x + 2} 184h3v3h-3z`}
                    fill="var(--gate-beacon)"
                  />
                  <path
                    d={`M${x + 16} 189h64v1h-64z`}
                    fill="var(--scene-signal)"
                    opacity=".5"
                  />
                </g>
              ))}
            </>
          )}
        </g>

        {trees.map(([x, y, scale]) => (
          <g
            key={x}
            transform={`translate(${x} ${y}) scale(${pastoral ? scale * 1.2 : scale})`}
          >
            <path d="M-2-4h4v30h-4z" fill="var(--scene-pole)" />
            <path
              d="M-27 10V-9h5v-13h8v-9h9v-6H5v5h10v8h7v12h6v21h-9v5H9V9H2v7h-9v-5h-10v5h-7v-6z"
              fill="var(--scene-foreground)"
            />
            <path
              d="M-21-10v-10h8v-8h10v-4H6v5h8v6h7v9h-7v-7H7v-5H-4v5h-9v9zM-19-6h2v12h-2zM13-10h2v14h-2zM4-16h2V3H4z"
              fill="var(--scene-foreground-edge)"
              opacity={pastoral ? ".7" : ".28"}
            />
          </g>
        ))}

        {/* A small terminal replaces the name stone; guide lights lead into the city. */}
        <path d="M0 222h600v28H0z" fill="var(--scene-foreground)" />
        <path
          d="M214 218h115l37 32H177zM0 232h181l-8 3H0zM377 233h223v2H381z"
          fill="var(--scene-road)"
        />
        <path
          d="M219 220h103m-135 25h163M19 228h114m326 0h128"
          stroke="var(--scene-reflection)"
          strokeOpacity=".5"
        />
        <path
          d="M251 212v-13h7v-5h28v3h9v15z"
          fill="var(--gate-shell-shadow)"
        />
        <path
          d="M258 199h29v2h-29zM265 203h17v1h-17z"
          fill="var(--scene-signal)"
          opacity=".8"
        />
        <path
          d="M184 219v-15h21v15m134 0v-20h25v20"
          fill="var(--scene-building-light)"
        />
        <path
          d="M181 202h27v3h-27zM336 197h31v3h-31z"
          fill="var(--scene-roof-edge)"
        />
        <path
          d="M188 207h11v7h-11zM344 203h14v9h-14z"
          fill="var(--gate-glass)"
        />
        <path d="M178 222v-21h2v21m187 0v-24h2v24" fill="var(--scene-pole)" />
        <path d="M178 199h3v3h-3zM367 196h3v3h-3z" fill="var(--scene-signal)" />
        <g stroke="var(--gate-pier)" strokeOpacity=".6">
          <path d="M214 213h114m-114 7h114" />
          {Array.from({ length: 28 }, (_, index) => (
            <path key={index} d={`M${216 + index * 4} 212v10`} />
          ))}
        </g>
        <path
          d="M235 231h6v2h-6zM230 241h8v2h-8zM306 232h6v2h-6zM311 242h8v2h-8z"
          fill="var(--scene-signal)"
          opacity=".75"
        />
        {!pastoral && (
          <>
            <path
              d="M208 228h133m-147 9h158m-65-15-7 28m20-28 13 28"
              stroke="var(--scene-reflection)"
              opacity=".2"
            />
            <path
              d="M396 232h39v1h-39zM453 239h28v1h-28zM503 231h58v1h-58zM511 237h19v1h-19z"
              fill="var(--scene-signal)"
              opacity=".35"
            />
          </>
        )}
        {pastoral && <PastoralPlantings />}
        <path
          className="campus-scene-fade"
          d="M0 0h600v250H0z"
          fill="url(#campus-fade)"
        />
      </g>
    </svg>
  );
}
