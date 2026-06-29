// SPDX-License-Identifier: GPL-3.0-only

// Package spec defines the core types that flow through scampi's pipeline.
//
// There are two vocabularies, and the prefix tells you which world a type
// belongs to:
//
//   - Declared* are the user's intent, exactly as the language linker emits
//     it. DeclaredConfig holds DeclaredDeploy blocks, each holding DeclaredStep
//     items; DeclaredTarget is referenced by name. Nothing here is resolved or
//     planned yet.
//
//   - Execution types are bare nouns: what the engine runs. A Plan wraps one
//     Deploy, which runs its Steps (each a parallel DAG of Ops) against a live
//     target.Target. They appear everywhere, so they keep short canonical
//     names.
//
// The pipeline crosses between worlds in two hops:
//
//	linker          -> DeclaredConfig                 (declare)
//	engine.Resolve  -> Config (one per deploy×target) (select)
//	StepKind.Plan   -> Plan{ Deploy{ Step{ Op } } }   (plan, then execute)
//
// Config is the hinge: a resolved selection (one deploy plus one target) whose
// Steps are still DeclaredStep. Planning is what turns each DeclaredStep into
// an executable Step.
//
// The spec layer is descriptive only and contains no execution logic. Types are
// grouped by world across files: resource.go (the promise/input surface),
// declared.go (linker output plus per-kind types), execution.go (the planned,
// runnable types), span.go (source locations).
package spec
