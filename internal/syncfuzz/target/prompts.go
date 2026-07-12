package target

import (
	_ "embed"
)

//go:embed prompts/file-residue-fork.txt
var FileResidueForkPrompt string

//go:embed prompts/file-residue-fork-veri.txt
var FileResidueForkVerificationPrompt string

//go:embed prompts/directory-residue-fork.txt
var DirectoryResidueForkPrompt string

//go:embed prompts/directory-residue-fork-veri.txt
var DirectoryResidueForkVerificationPrompt string

//go:embed prompts/delete-residue-fork.txt
var DeleteResidueForkPrompt string

//go:embed prompts/delete-residue-fork-veri.txt
var DeleteResidueForkVerificationPrompt string

//go:embed prompts/symlink-residue-fork.txt
var SymlinkResidueForkPrompt string

//go:embed prompts/symlink-residue-fork-veri.txt
var SymlinkResidueForkVerificationPrompt string

//go:embed prompts/rename-residue-fork.txt
var RenameResidueForkPrompt string

//go:embed prompts/rename-residue-fork-veri.txt
var RenameResidueForkVerificationPrompt string

//go:embed prompts/mode-residue-fork.txt
var ModeResidueForkPrompt string

//go:embed prompts/mode-residue-fork-veri.txt
var ModeResidueForkVerificationPrompt string

//go:embed prompts/append-residue-fork.txt
var AppendResidueForkPrompt string

//go:embed prompts/append-residue-fork-veri.txt
var AppendResidueForkVerificationPrompt string

//go:embed prompts/hardlink-residue-fork.txt
var HardlinkResidueForkPrompt string

//go:embed prompts/hardlink-residue-fork-veri.txt
var HardlinkResidueForkVerificationPrompt string

//go:embed prompts/fifo-residue-fork.txt
var FifoResidueForkPrompt string

//go:embed prompts/fifo-residue-fork-veri.txt
var FifoResidueForkVerificationPrompt string

//go:embed prompts/openfd-residue-fork.txt
var OpenFDResidueForkPrompt string

//go:embed prompts/openfd-residue-fork-veri.txt
var OpenFDResidueForkVerificationPrompt string

//go:embed prompts/deleted-openfd-fork.txt
var DeletedOpenFDResidueForkPrompt string

//go:embed prompts/deleted-openfd-fork-veri.txt
var DeletedOpenFDResidueForkVerificationPrompt string

//go:embed prompts/inherited-fd-leak.txt
var InheritedFDLeakPrompt string

//go:embed prompts/inherited-fd-leak-veri.txt
var InheritedFDLeakVerificationPrompt string

//go:embed prompts/unix-listener-residue-fork.txt
var UnixListenerResidueForkPrompt string

//go:embed prompts/unix-listener-residue-fork-veri.txt
var UnixListenerResidueForkVerificationPrompt string

//go:embed prompts/discarded-server-trusted-client.txt
var DiscardedServerTrustedClientPrompt string

//go:embed prompts/discarded-server-trusted-client-veri.txt
var DiscardedServerTrustedClientVerificationPrompt string

//go:embed prompts/socket-response-poisoning.txt
var SocketResponsePoisoningPrompt string

//go:embed prompts/socket-response-poisoning-veri.txt
var SocketResponsePoisoningVerificationPrompt string

//go:embed prompts/cwd-residue.txt
var CWDResiduePrompt string

//go:embed prompts/env-residue.txt
var EnvResiduePrompt string

//go:embed prompts/function-residue.txt
var FunctionResiduePrompt string

//go:embed prompts/cwd-residue-fork.txt
var CWDResidueForkPrompt string

//go:embed prompts/cwd-residue-fork-veri.txt
var CWDResidueForkVerificationPrompt string

//go:embed prompts/umask-residue.txt
var UmaskResiduePrompt string

//go:embed prompts/umask-residue-fork.txt
var UmaskResidueForkPrompt string

//go:embed prompts/umask-residue-fork-veri.txt
var UmaskResidueForkVerificationPrompt string

//go:embed prompts/long-delay.txt
var LongDelayPrompt string

//go:embed prompts/maf-orphan-process.txt
var MAFOrphanProcessPrompt string

//go:embed prompts/maf-persistent-shell.txt
var MAFPersistentShellPrompt string

//go:embed prompts/persistent-shell.txt
var PersistentShellPrompt string

//go:embed prompts/persistent-shell-replay.txt
var PersistentShellReplayPrompt string

//go:embed prompts/persistent-shell-fork.txt
var PersistentShellForkPrompt string
