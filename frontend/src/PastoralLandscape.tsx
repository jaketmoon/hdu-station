// Original pixel scenery for the warm theme, drawn in the gate's SVG coordinates.
export function PastoralBackdrop() {
  return (
    <g className="pastoral-backdrop">
      <path
        d="M354 27h20v4h8v8h4v20h-4v8h-8v4h-20v-4h-8v-8h-4V39h4v-8h8z"
        fill="var(--pastoral-sun)"
      />
      <g fill="var(--scene-haze)" opacity=".8">
        <path d="M111 63h17v-7h27v-6h29v6h25v7h13v7H111zM282 75h18v-6h30v-7h24v7h17v6h13v7H282zM390 41h17v-6h27v-5h21v5h15v6h13v6h-93z" />
      </g>
      <path
        d="M0 171h33v-10h28v-11h30v-9h35v-10h33v-13h29v-11h28v-9h37v11h26v12h26v9h34v12h34v-8h26v-15h24v-13h26V93h31v11h29v11h27v15h28v12h23v14h28v61H0z"
        fill="var(--pastoral-hill-far)"
      />
      <path
        d="M0 191h46v-11h47v-9h44v-12h31v-10h37v-9h37v9h40v11h35v12h43v-9h28v-12h28v-10h33v-9h31v9h34v11h35v10h34v11h27v48H0z"
        fill="var(--pastoral-hill-near)"
      />
      <path
        d="M0 201h69v-6h67v-6h94v8h94v-7h93v7h104v-9h79v44H0z"
        fill="var(--pastoral-meadow)"
      />
      {[
        [167, 165],
        [207, 156],
        [398, 154],
        [457, 148],
        [586, 153],
      ].map(([x, y]) => (
        <g key={x} transform={`translate(${x} ${y})`}>
          <path d="M-2 5h4v24h-4z" fill="var(--scene-pole)" />
          <path
            d="M-4-28h8v7h6v7h6v7h5v8h-8l11 13h-48l11-13h-8v-8h5v-7h6v-7h6z"
            fill="var(--pastoral-leaf)"
          />
          <path
            d="M-4-28h5v9h-5v7h-6v7h-7v6h-4v-8h5v-7h6v-7h6zM-12 5h7v3h-7z"
            fill="var(--pastoral-leaf-light)"
            opacity=".7"
          />
        </g>
      ))}
      {/* Warm timber cottages sit behind the entrance bridge. */}
      {[
        [249, 167, 0.8],
        [371, 154, 1],
      ].map(([x, y, scale]) => (
        <g key={x} transform={`translate(${x} ${y}) scale(${scale})`}>
          <path d="M0 0h54v46H0z" fill="var(--scene-building-light)" />
          <path d="M45-30h7v24h-7z" fill="var(--gate-shell-shadow)" />
          <path
            d="M-8 0v-6h7v-6h7v-6h7v-6h7v-6h14v6h7v6h7v6h7v6h7v6z"
            fill="var(--pastoral-roof)"
          />
          <path
            d="M-8 0h70v3H-8zM6-12h42v2H6zM20-24h14v2H20z"
            fill="var(--pastoral-roof-light)"
          />
          <path
            d="M1 13h52m-52 13h52m-52 13h52"
            stroke="var(--scene-building-edge)"
            opacity=".5"
          />
          <path
            d="M22 17h12v29H22zM7 13h10v13H7zM39 13h10v13H39z"
            fill="var(--gate-shell-shadow)"
          />
          <path
            d="M8 14h8v11H8zM40 14h8v11h-8z"
            fill="var(--scene-window-warm)"
          />
          <path
            d="M12 14v11m-4-6h8m28-5v11m-4-6h8"
            stroke="var(--pastoral-roof)"
          />
          <path d="M19 46h18v3H19z" fill="var(--gate-pier-light)" />
          <path d="M31 31h1v2h-1z" fill="var(--scene-window-warm)" />
        </g>
      ))}
      {/* A still windmill adds a farm silhouette without motion behind the text. */}
      <g transform="translate(323 121)">
        <path d="M-9 3h18v45h4v9h-26v-9h4z" fill="var(--gate-pier)" />
        <path
          d="M-13 4v-5h5v-5h5v-5h6v5h5v5h5v5z"
          fill="var(--pastoral-roof)"
        />
        <path
          d="M-8 18H8m-16 13H8m-16 12H8"
          stroke="var(--gate-pier-shadow)"
          opacity=".45"
        />
        <path d="M-3 41h6v16h-6z" fill="var(--gate-shell-shadow)" />
        {[0, 90, 180, 270].map((angle) => (
          <g key={angle} transform={`rotate(${angle})`}>
            <path
              d="M1-29h7v20H1z"
              fill="var(--pastoral-petal)"
              stroke="var(--gate-truss)"
            />
            <path d="M1-23h7m-7 6h7M0-9V0" stroke="var(--gate-truss)" />
          </g>
        ))}
        <path d="M-3-3h6v6h-6z" fill="var(--pastoral-roof)" />
      </g>
      <path d="M222 202h113v17H222z" fill="var(--scene-road)" />
      <path
        d="M228 207h101m-101 7h101"
        stroke="var(--gate-truss)"
        opacity=".5"
      />
      {Array.from({ length: 12 }, (_, index) => (
        <path
          key={index}
          d={`M${228 + index * 9} 202v5m-3-7h3v3m0-5h3v3`}
          stroke="var(--pastoral-leaf)"
        />
      ))}
    </g>
  );
}

export function PastoralPlantings() {
  return (
    <g>
      {/* Ivy follows the tower edge, leaving the circular opening clear. */}
      <path
        d="M568 70h-4v23h-5v29h4v25h-7v29h-7v27m-79-24h12v18h5v20M324 160v14h-10v12m-23-24v12h-7"
        stroke="var(--pastoral-leaf)"
        strokeWidth="2"
      />
      <path
        d="M557 78h7v5h-7zM563 89h7v5h-7zM553 105h7v5h-7zM559 118h8v6h-8zM557 142h7v6h-7zM549 162h8v5h-8zM552 178h7v5h-7zM543 190h8v5h-8zM472 181h9v5h-9zM480 199h8v6h-8zM315 168h8v5h-8zM308 179h7v4h-7zM283 170h8v5h-8z"
        fill="var(--pastoral-leaf)"
      />
      <path
        d="M558 77h5v2h-5zM554 104h5v2h-5zM550 161h6v2h-6zM544 189h6v2h-6zM481 198h6v2h-6z"
        fill="var(--pastoral-leaf-light)"
      />
      {[
        [87, 232],
        [135, 226],
        [169, 239],
        [353, 237],
        [397, 229],
        [451, 238],
        [479, 230],
        [552, 239],
        [582, 226],
      ].map(([x, y], index) => (
        <g key={x} transform={`translate(${x} ${y})`}>
          <path d="M0-3v8m0-1h-4v-3m4 1h4v-3" stroke="var(--pastoral-leaf)" />
          <path
            d="M-2-8h4v3h3v4H2v3h-4v-3h-3v-4h3z"
            fill={
              index % 3 === 0
                ? "var(--pastoral-flower)"
                : "var(--pastoral-petal)"
            }
          />
          <path d="M-1-4h2v2h-2z" fill="var(--gate-truss)" />
        </g>
      ))}
      <path
        d="M68 240v-4m-3 5v-3m54-5v-4m-4 6v-3m76 10v-4m-4 6v-3m202-5v-5m-4 7v-4m103-4v-4m-4 7v-4m78 1v-5"
        stroke="var(--pastoral-leaf-light)"
      />
    </g>
  );
}
