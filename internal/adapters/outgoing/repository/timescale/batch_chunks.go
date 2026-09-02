package timescale

// chunkSymbols parte symbols en lotes de a lo sumo size -- las consultas de
// lote sobre el universo entero (GetIntradaySessionsBatch) agregan por
// symbol_id con ARRAY_AGG, que materializa un array por grupo antes de
// recortarlo a un solo valor. Contra los ~13k simbolos activos de una sola
// pasada, eso infla la conexion que corre la consulta a mas de 1GB de RSS
// que Postgres despues nunca devuelve (confirmado en vivo el 2026-09-02,
// ver MaxConnLifetime en storage/timescale.go) -- partir en lotes mas chicos
// acota el pico de memoria por consulta sin cambiar el resultado final.
func chunkSymbols(symbols []string, size int) [][]string {
	if size <= 0 || len(symbols) <= size {
		return [][]string{symbols}
	}
	chunks := make([][]string, 0, (len(symbols)+size-1)/size)
	for i := 0; i < len(symbols); i += size {
		end := min(i+size, len(symbols))
		chunks = append(chunks, symbols[i:end])
	}
	return chunks
}
