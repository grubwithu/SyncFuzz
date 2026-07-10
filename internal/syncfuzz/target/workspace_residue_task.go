package target

type workspaceResidueTaskSpec struct {
	TaskID                  string
	Description             string
	Objective               string
	Prompt                  string
	ExpectedFiles           []string
	CheckpointSelector      string
	ForkVerificationMessage string
}

func workspaceResidueTaskSpecs() []workspaceResidueTaskSpec {

	return []workspaceResidueTaskSpec{
		{
			TaskID:                  FileResidueForkTargetTaskID,
			Description:             "fork from a pre-write checkpoint and observe whether branch-note.txt still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-write checkpoint still sees a workspace file that should have disappeared.",
			Prompt:                  FileResidueForkPrompt,
			ExpectedFiles:           []string{TargetFileResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-file-drop",
			ForkVerificationMessage: FileResidueForkVerificationPrompt,
		},
		{
			TaskID:                  DirectoryResidueForkTargetTaskID,
			Description:             "fork from a pre-directory checkpoint and observe whether branch-dir still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-directory checkpoint still sees a workspace directory that should have disappeared.",
			Prompt:                  DirectoryResidueForkPrompt,
			ExpectedFiles:           []string{TargetDirectoryResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-directory-create",
			ForkVerificationMessage: DirectoryResidueForkVerificationPrompt,
		},
		{
			TaskID:                  DeleteResidueForkTargetTaskID,
			Description:             "fork from a pre-delete checkpoint and observe whether branch-delete-note.txt wrongly stays absent in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-delete checkpoint still sees a workspace file as absent even though the checkpoint predates deletion.",
			Prompt:                  DeleteResidueForkPrompt,
			ExpectedFiles:           []string{TargetDeleteResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-file-delete",
			ForkVerificationMessage: DeleteResidueForkVerificationPrompt,
		},
		{
			TaskID:                  SymlinkResidueForkTargetTaskID,
			Description:             "fork from a pre-symlink checkpoint and observe whether branch-link.txt still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-symlink checkpoint still sees a workspace symlink that should have disappeared.",
			Prompt:                  SymlinkResidueForkPrompt,
			ExpectedFiles:           []string{TargetSymlinkResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-symlink-create",
			ForkVerificationMessage: SymlinkResidueForkVerificationPrompt,
		},
		{
			TaskID:                  RenameResidueForkTargetTaskID,
			Description:             "fork from a pre-rename checkpoint and observe whether the post-rename destination file still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-rename checkpoint still sees the renamed destination file instead of the original source name.",
			Prompt:                  RenameResidueForkPrompt,
			ExpectedFiles:           []string{TargetRenameResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-file-rename",
			ForkVerificationMessage: RenameResidueForkVerificationPrompt,
		},
		{
			TaskID:                  ModeResidueForkTargetTaskID,
			Description:             "fork from a pre-chmod checkpoint and observe whether the tightened file mode still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-chmod checkpoint still sees a later file-permission change in the workspace.",
			Prompt:                  ModeResidueForkPrompt,
			ExpectedFiles:           []string{TargetModeResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-file-chmod",
			ForkVerificationMessage: ModeResidueForkVerificationPrompt,
		},
		{
			TaskID:                  AppendResidueForkTargetTaskID,
			Description:             "fork from a pre-append checkpoint and observe whether later appended content still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-append checkpoint still sees later file-content growth in the workspace.",
			Prompt:                  AppendResidueForkPrompt,
			ExpectedFiles:           []string{TargetAppendResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-file-append",
			ForkVerificationMessage: AppendResidueForkVerificationPrompt,
		},
		{
			TaskID:                  HardlinkResidueForkTargetTaskID,
			Description:             "fork from a pre-hardlink checkpoint and observe whether a later hardlink still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-hardlink checkpoint still sees a later workspace hardlink.",
			Prompt:                  HardlinkResidueForkPrompt,
			ExpectedFiles:           []string{TargetHardlinkResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-hardlink-create",
			ForkVerificationMessage: HardlinkResidueForkVerificationPrompt,
		},
		{
			TaskID:                  FifoResidueForkTargetTaskID,
			Description:             "fork from a pre-fifo checkpoint and observe whether a later named pipe still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-fifo checkpoint still sees a later workspace named pipe.",
			Prompt:                  FifoResidueForkPrompt,
			ExpectedFiles:           []string{TargetFIFOResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-fifo-create",
			ForkVerificationMessage: FifoResidueForkVerificationPrompt,
		},
		{
			TaskID:                  OpenFDResidueForkTargetTaskID,
			Description:             "fork from a pre-open-fd checkpoint and observe whether a later background fd holder still exists",
			Objective:               "Observe whether LangGraph fork from a pre-open-fd checkpoint still sees a later process that keeps a workspace file descriptor open.",
			Prompt:                  OpenFDResidueForkPrompt,
			ExpectedFiles:           []string{TargetOpenFDResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-open-fd-hold",
			ForkVerificationMessage: OpenFDResidueForkVerificationPrompt,
		},
		{
			TaskID:                  DeletedOpenFDForkTargetTaskID,
			Description:             "fork from a pre-deleted-open-fd checkpoint and observe whether a deleted workspace inode still stays reachable through a later background fd holder",
			Objective:               "Observe whether LangGraph fork from a pre-deleted-open-fd checkpoint still sees a later process that keeps a deleted workspace inode open.",
			Prompt:                  DeletedOpenFDResidueForkPrompt,
			ExpectedFiles:           []string{TargetDeletedOpenFDForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-deleted-open-fd-hold",
			ForkVerificationMessage: DeletedOpenFDResidueForkVerificationPrompt,
		},
		{
			TaskID:                  InheritedFDLeakTargetTaskID,
			Description:             "fork from a pre-fd-leak checkpoint and observe whether a successor branch can read a discarded branch secret through an inherited fd",
			Objective:               "Observe whether LangGraph fork from before a branch-local fd holder still lets the successor branch read discarded branch data through /proc/<pid>/fd/9.",
			Prompt:                  InheritedFDLeakPrompt,
			ExpectedFiles:           []string{TargetInheritedFDLeakForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-inherited-fd-leak-holder",
			ForkVerificationMessage: InheritedFDLeakVerificationPrompt,
		},
		{
			TaskID:                  UnixListenerResidueForkTargetTaskID,
			Description:             "fork from a pre-listener checkpoint and observe whether a branch-local Unix socket listener still answers in the successor branch",
			Objective:               "Observe whether LangGraph fork from before a branch-local Unix listener still lets the successor branch connect to the discarded branch service.",
			Prompt:                  UnixListenerResidueForkPrompt,
			ExpectedFiles:           []string{TargetUnixListenerForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-unix-listener-launch",
			ForkVerificationMessage: UnixListenerResidueForkVerificationPrompt,
		},
		{
			TaskID:                  CWDResidueForkTargetTaskID,
			Description:             "fork from a pre-cwd checkpoint and observe whether a branch-local cwd still answers in the successor branch",
			Objective:               "Observe whether LangGraph fork from before a branch-local cwd still lets the successor branch access the discarded branch cwd.",
			Prompt:                  CWDResidueForkPrompt,
			ExpectedFiles:           []string{TargetCWDResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-cwd-change",
			ForkVerificationMessage: CWDResidueForkVerificationPrompt,
		},
		{
			TaskID:                  UmaskResidueForkTargetTaskID,
			Description:             "fork from a pre-umask checkpoint and observe whether a branch-local umask still answers in the successor branch",
			Objective:               "Observe whether LangGraph fork from before a branch-local umask still lets the successor branch set the umask to the discarded branch umask.",
			Prompt:                  UmaskResidueForkPrompt,
			ExpectedFiles:           []string{TargetUmaskResidueForkArtifact, LanggraphForkArtifact},
			CheckpointSelector:      "before-umask-change",
			ForkVerificationMessage: UmaskResidueForkVerificationPrompt,
		},
	}
}

func WorkspaceResidueTaskSpecByID(taskID string) (workspaceResidueTaskSpec, bool) {
	for _, spec := range workspaceResidueTaskSpecs() {
		if spec.TaskID == taskID {
			return spec, true
		}
	}
	return workspaceResidueTaskSpec{}, false
}

func workspaceResidueTaskIDs() []string {
	specs := workspaceResidueTaskSpecs()
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.TaskID)
	}
	return ids
}
