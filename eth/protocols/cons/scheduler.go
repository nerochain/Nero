package cons

// AttestationScheduler schedule attestation processing task,
// it's quite like the role of blockFetcher to blocks.
type AttestationScheduler struct {
	backend Backend
}
