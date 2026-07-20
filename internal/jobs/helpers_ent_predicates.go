package jobs

import (
	entsql "entgo.io/ent/dialect/sql"

	"kv-shepherd.io/shepherd/ent/predicate"
)

// lockTicketRowForUpdate is the reviewed raw-Ent predicate used by parent
// aggregation. The lock must be acquired before child rows are read so a stale
// aggregator waits for a concurrent retry transaction and observes its
// committed child state under READ COMMITTED.
func lockTicketRowForUpdate() predicate.Ticket {
	return func(selector *entsql.Selector) {
		selector.ForUpdate()
	}
}

// lockDomainEventRowForUpdate is the reviewed raw-Ent predicate used after
// child tickets have been locked in deterministic ID order. Keeping the event
// lock as a separate predicate makes the children -> events -> parent order
// explicit for batch decision transactions.
func lockDomainEventRowForUpdate() predicate.DomainEvent {
	return func(selector *entsql.Selector) {
		selector.ForUpdate()
	}
}

// lockVMRowForUpdate keeps the immutable provider coordinates and deleting
// state stable while a power event wins its durable dispatch claim.
func lockVMRowForUpdate() predicate.VM {
	return func(selector *entsql.Selector) {
		selector.ForUpdate()
	}
}

// lockBatchTicketRowForUpdate keeps the parent power-batch projection stable
// while a child worker proves that EXECUTING is legitimate batch provenance.
func lockBatchTicketRowForUpdate() predicate.BatchTicket {
	return func(selector *entsql.Selector) {
		selector.ForUpdate()
	}
}
