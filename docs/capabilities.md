# Reference capabilities

`capabilities/` holds presets for common machines, one list per process, to show whether a design can be made on typical equipment. They are never applied on their own: pass one as a capability (`--capability preset:<id>`, `capabilities.Get(id)`; `dfm capabilities` lists them). Each list's `index.json` gives every preset's version, sources and how strong the evidence is. A shop's own capability adds its stocked thicknesses, hole operations and policies.

**Laser cutters**

| Preset                            | Machine                                               | Usable area    | Materials (maximum thickness)                                          |
| --------------------------------- | ----------------------------------------------------- | -------------- | ---------------------------------------------------------------------- |
| `fiber-2kw-3015`                  | Fiber, 2 kW (AMADA FOL 3015 AJ data)                  | 3000 × 1500 mm | mild steel 15.88, stainless 10.01, aluminum 8, copper 3.99, brass 5 mm |
| `fiber-3kw-3015`                  | Fiber, 3 kW (Bystronic ByCut Fiber 3000)              | 3000 × 1500 mm | 19.05, 11.94, 11.94, 6.35, 6.35 mm                                     |
| `fiber-6kw-3015`                  | Fiber, 6 kW (Bystronic ByCut Fiber 6000)              | 3000 × 1500 mm | 25.4, 29.97, 29.97, 11.94, 14.99 mm                                    |
| `fiber-12kw-4020`                 | Fiber, 12 kW (Bystronic ByCut Fiber 12000)            | 4000 × 2000 mm | as 6 kW                                                                |
| `co2-desktop`                     | Desktop CO2 (Glowforge Pro class)                     | 495 × 279 mm   | acrylic, plywood, MDF 6.35 mm                                          |
| `co2-100w-1300x900`               | CO2, 100 W                                            | 1300 × 900 mm  | acrylic 12, acetal 6, plywood 9, MDF 9 mm                              |
| `co2-150w-4x8`                    | CO2, 150 W, 4 × 8 ft                                  | 2438 × 1219 mm | acrylic 25.4, plywood 12.7, MDF 12.7 mm                                |
| `trumpf-trulaser-1030-fiber-3kw`  | TRUMPF TruLaser 1030 fiber, 3 kW                      | 3000 × 1500 mm | mild steel 20, stainless 15, aluminum 15, copper 6, brass 6 mm         |
| `trumpf-trulaser-1030-fiber-6kw`  | TRUMPF TruLaser 1030 fiber, 6 kW                      | 3000 × 1500 mm | 25, 25, 25, 10, 10 mm                                                  |
| `trumpf-trulaser-3030-fiber-10kw` | TRUMPF TruLaser 3030 fiber, 10 kW                     | 3000 × 1500 mm | 25, 30, 25, 15, 12 mm                                                  |
| `amada-ventis-3015aje-4kw`        | AMADA VENTIS-3015AJe, 4 kW                            | 3000 × 1500 mm | 25, 20, 15, 8, 10 mm                                                   |
| `amada-ensis-3015-12kw`           | AMADA ENSIS 3015, 12 kW (indicative data)             | 3070 × 1550 mm | 25, 25, 25, 12, 18 mm                                                  |
| `mazak-optiplex-3015-nexus-fiber` | Mazak OPTIPLEX 3015 NEXUS fiber, 6–8 kW (dealer data) | 3000 × 1500 mm | 25.4, 25.4, 19.05 mm; no copper or brass                               |

Fiber presets share the limits of `fixtures/capabilities/capability-metal.json`, CO2 presets those of `capability-nonmetal.json` (a test enforces it).

**Press brakes**

| Preset                     | Machine                  | Press force | Bending length |
| -------------------------- | ------------------------ | ----------- | -------------- |
| `trumpf-trubend-7036`      | TRUMPF TruBend 7036      | 360 kN      | 1020 mm        |
| `amada-eg-6013`            | AMADA EG-6013            | 600 kN      | 1400 mm        |
| `trumpf-trubend-3100`      | TRUMPF TruBend 3100      | 1000 kN     | 3060 mm        |
| `trumpf-trubend-5130`      | TRUMPF TruBend 5130      | 1300 kN     | 3230 mm        |
| `amada-hg-1303`            | AMADA HG-1303            | 1300 kN     | 3109 mm        |
| `bystronic-xpert-150-3100` | Bystronic Xpert 150/3100 | 1500 kN     | 3100 mm        |

Each bends mild steel, 304 stainless and 5052 aluminum (6061 cracks), with one shared set of bending limits based on the stock thickness t and inside radius R: flange 2 t + R, relief width max(0.4 mm, t/2), and the default K-factor of 0.44. Maximum thicknesses are conservative; the force check is the real limit for long bends.
