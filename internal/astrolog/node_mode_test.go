package astrolog

import (
	"context"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// The node mode is the lens's, whatever a host astrolog.as says: a chart cast
// for a true-node lens prints the same North Node under a host that sets the
// mean node and under one that sets the true node. A bare `-Yn` would flip the
// host's setting and print two different nodes.
func TestNodeModeIgnoresTheHostSetting(t *testing.T) {
	needAstrolog(t)
	node := func() float64 {
		out, err := ComputeChartPositionsOnly(context.Background(), hellenisticSpec(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range out.Positions {
			if p.Object.Name == "North Node" {
				return p.Longitude
			}
		}
		t.Fatalf("no North Node in %v", out.Positions)
		return 0
	}
	useDataDir(t, dataDirWith(t, "_Yn"))
	underMean := node()
	useDataDir(t, dataDirWith(t, "=Yn"))
	underTrue := node()
	if underMean != underTrue {
		t.Errorf("the node follows the host's setting: %.4f under a host _Yn, %.4f under a host =Yn", underMean, underTrue)
	}
}

// Lilith is the osculating apogee whatever a host astrolog.as says: Astrolog's
// natural node and Lilith (`=Ynn`) outrank the true ones (calc.cpp:2594), so
// Sextile clears them (`_Ynn`) beside the node mode. Under a host that sets
// =Ynn the chart's Lilith is the one Astrolog casts with =Yn _Ynn and no host
// setting: the true Lilith the reference names.
func TestLilithIsTheOsculatingApogeeUnderAHostNaturalNode(t *testing.T) {
	needAstrolog(t)
	lilith := func() float64 {
		spec := hellenisticSpec(t)
		spec.Objects.Nodes = recipe.NodesTrue
		out, err := ComputeChartPositionsOnly(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range out.Positions {
			if p.Object.Name == "Lilith" {
				return p.Longitude
			}
		}
		t.Fatalf("no Lilith in %v", out.Positions)
		return 0
	}
	useDataDir(t, dataDirWith(t, "_Ynn"))
	plain := lilith()
	useDataDir(t, dataDirWith(t, "=Ynn"))
	natural := lilith()
	if plain != natural {
		t.Errorf("Lilith follows a host =Ynn: %.4f with it, %.4f without (the osculating apogee)", natural, plain)
	}
}
