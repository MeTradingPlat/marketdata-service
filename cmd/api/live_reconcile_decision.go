package main

// shouldSkipReconcileRetry decide si el reconciler se salta un simbolo en
// este tick. Mientras el rollout inicial de la ventana actual sigue en curso
// (!rolloutDone), un simbolo nunca intentado se deja en paz -- reintentarlo
// aca chocaria con el propio rollout que ya lo va a alcanzar (confirmado en
// vivo el 2026-08-18: doble registro del dispatch entry freno el ciclo 15+
// min). Una vez que el rollout de esta ventana ya termino, un simbolo que
// sigue sin intentar solo pudo quedar afuera de la foto de tracked/
// activeTracked por una falla silenciosa de la consulta masiva (ver
// seedRetryDelay en universe_cycle.go) -- ahi hay que tratarlo como
// cualquier otro caido, o se queda mudo hasta el proximo reinicio completo
// del proceso (confirmado en vivo el 2026-09-01: TWO, LEG, RMAX y ~200
// simbolos liquidos mas nunca aparecieron ni intentados ni fallidos tras el
// reinicio, sin autosanar).
func shouldSkipReconcileRetry(attempted, rolloutDone, live, subscribed bool) bool {
	if !attempted {
		return !rolloutDone
	}
	return live && subscribed
}
