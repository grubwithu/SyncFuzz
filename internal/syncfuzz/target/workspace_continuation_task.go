package target

type workspaceContinuationTaskSpec struct {
	TaskID        string
	Description   string
	Objective     string
	Prompt        string
	ExpectedFiles []string
}

func workspaceContinuationTaskSpecs() []workspaceContinuationTaskSpec {
	return []workspaceContinuationTaskSpec{
		{
			TaskID:        FileResidueTargetTaskID,
			Description:   "create a workspace file in one shell call and observe whether it still exists in a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a created file across later shell calls.",
			Prompt:        FileResiduePrompt,
			ExpectedFiles: []string{TargetFileResidueCheckArtifact},
		},
		{
			TaskID:        DirectoryResidueTargetTaskID,
			Description:   "create a workspace directory in one shell call and observe whether it still exists in a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a created directory across later shell calls.",
			Prompt:        DirectoryResiduePrompt,
			ExpectedFiles: []string{TargetDirectoryResidueCheckArtifact},
		},
		{
			TaskID:        DeleteResidueTargetTaskID,
			Description:   "create and delete a workspace file, then observe whether that deleted state persists into a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a file-deletion state across later shell calls.",
			Prompt:        DeleteResiduePrompt,
			ExpectedFiles: []string{TargetDeleteResidueCheckArtifact},
		},
		{
			TaskID:        SymlinkResidueTargetTaskID,
			Description:   "create a workspace symlink in one shell call and observe whether it still exists in a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a created symlink across later shell calls.",
			Prompt:        SymlinkResiduePrompt,
			ExpectedFiles: []string{TargetSymlinkResidueCheckArtifact},
		},
		{
			TaskID:        RenameResidueTargetTaskID,
			Description:   "rename a workspace file and observe whether the renamed binding persists into a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a rename result across later shell calls.",
			Prompt:        RenameResiduePrompt,
			ExpectedFiles: []string{TargetRenameResidueCheckArtifact},
		},
		{
			TaskID:        ModeResidueTargetTaskID,
			Description:   "tighten a workspace file mode and observe whether that mode persists into a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a file-mode change across later shell calls.",
			Prompt:        ModeResiduePrompt,
			ExpectedFiles: []string{TargetModeResidueCheckArtifact},
		},
		{
			TaskID:        AppendResidueTargetTaskID,
			Description:   "append to a workspace file and observe whether the appended content persists into a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves appended file content across later shell calls.",
			Prompt:        AppendResiduePrompt,
			ExpectedFiles: []string{TargetAppendResidueCheckArtifact},
		},
		{
			TaskID:        HardlinkResidueTargetTaskID,
			Description:   "create a workspace hardlink and observe whether it still exists in a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a created hardlink across later shell calls.",
			Prompt:        HardlinkResiduePrompt,
			ExpectedFiles: []string{TargetHardlinkResidueCheckArtifact},
		},
		{
			TaskID:        FifoResidueTargetTaskID,
			Description:   "create a workspace FIFO and observe whether it still exists in a later shell call",
			Objective:     "Observe whether a real workspace-backed target preserves a created FIFO across later shell calls.",
			Prompt:        FifoResiduePrompt,
			ExpectedFiles: []string{TargetFIFOResidueCheckArtifact},
		},
	}
}

func WorkspaceContinuationTaskSpecByID(taskID string) (workspaceContinuationTaskSpec, bool) {
	for _, spec := range workspaceContinuationTaskSpecs() {
		if spec.TaskID == taskID {
			return spec, true
		}
	}
	return workspaceContinuationTaskSpec{}, false
}

func workspaceContinuationTaskIDs() []string {
	specs := workspaceContinuationTaskSpecs()
	ids := make([]string, 0, len(specs))
	for _, spec := range specs {
		ids = append(ids, spec.TaskID)
	}
	return ids
}
