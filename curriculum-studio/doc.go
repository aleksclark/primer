// Package curriculumstudio is the root of the Curriculum Studio Go module.
//
// Module path (frozen): github.com/aleksclark/primer/curriculum-studio
// Physical root: curriculum-studio/
//
// S1 delivers the process shell: cmd/studio-server, STUDIO_ config, structured
// logging, /studio/v1 health+ready, embedded migrate on Studio DB only.
// Domain CRUD, authz, SPA, and gRPC land in later S* waves.
package curriculumstudio
