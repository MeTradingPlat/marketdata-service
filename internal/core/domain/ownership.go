package domain

// InsiderOwnership es la tenencia agregada de insiders (Form 3/4/5) para un
// simbolo, mas los CIK de esos owners -- necesarios para no restar dos veces
// la posicion de un holder 5%+ que tambien sea insider (ej. un founder-CEO)
// al calcular floatShares real.
type InsiderOwnership struct {
	Shares    int64
	OwnerCiks map[string]bool
}
