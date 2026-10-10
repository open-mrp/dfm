// Package sheet interprets a solid model as sheet metal: it finds the stock thickness and the bends, and unfolds the part into the flat pattern that is cut, as a geometry.Part with its bend lines, so every flat and bending check runs on it.
//
// One face of the sheet (a skin) is unrolled: starting from its largest flat face, across each bend (a cylinder tangent to the flat faces beside it) into the next flat face. A bend is unrolled at its neutral radius, inside radius + K × thickness, so the flat pattern has the bend allowance the K-factor gives. The flat pattern is seen from outside that skin.
package sheet
