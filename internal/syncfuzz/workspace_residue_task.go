package syncfuzz

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
			TaskID:                  fileResidueForkTargetTaskID,
			Description:             "fork from a pre-write checkpoint and observe whether branch-note.txt still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-write checkpoint still sees a workspace file that should have disappeared.",
			Prompt:                  "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create branch-note.txt in the current working directory with exactly this one-line content:\n   SYNCFUZZ_FILE_RESIDUE_MARKER\n2. In a later shell step, print branch-note.txt to confirm that it exists.\n3. Do not create file-residue-fork-check.txt during this initial branch. A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.\n4. Do not delete, rename, or modify branch-note.txt after creating it.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the file-creation step.\n2. Create branch-note.txt exactly once during the initial run.\n3. Do not store the marker in helper files, shell init files, `/tmp`, `/var/tmp`, `$HOME`, or other external paths.\n4. Leave the workspace ready for a later follow-up message that will only observe whether branch-note.txt still exists from a forked checkpoint.\n5. After finishing, briefly explain that branch-note.txt was created.",
			ExpectedFiles:           []string{targetFileResidueForkArtifact, langgraphForkArtifact},
			CheckpointSelector:      "before-file-drop",
			ForkVerificationMessage: "This is the later SyncFuzz fork-observation step, so the earlier instruction about not creating file-residue-fork-check.txt no longer applies. Without recreating or modifying branch-note.txt, use exactly one shell tool call to run: if [ -f branch-note.txt ]; then cat branch-note.txt > file-residue-fork-check.txt; else printf 'MISSING_BRANCH_NOTE\\n' > file-residue-fork-check.txt; fi; cat file-residue-fork-check.txt. After that, briefly explain whether branch-note.txt already existed in this fork.",
		},
		{
			TaskID:                  directoryResidueForkTargetTaskID,
			Description:             "fork from a pre-directory checkpoint and observe whether branch-dir still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-directory checkpoint still sees a workspace directory that should have disappeared.",
			Prompt:                  "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create a directory named branch-dir in the current working directory.\n2. In a later shell step, confirm that branch-dir exists by using a command in the style of:\n   ls -ld branch-dir\n3. Do not create directory-residue-fork-check.txt during this initial branch. A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.\n4. Do not delete, rename, or modify branch-dir after creating it.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the directory-creation step.\n2. Create branch-dir exactly once during the initial run.\n3. Do not store helper state in `/tmp`, `/var/tmp`, `$HOME`, shell init files, or other external paths.\n4. Leave the workspace ready for a later follow-up message that will only observe whether branch-dir still exists from a forked checkpoint.\n5. After finishing, briefly explain that branch-dir was created.",
			ExpectedFiles:           []string{targetDirectoryResidueForkArtifact, langgraphForkArtifact},
			CheckpointSelector:      "before-directory-create",
			ForkVerificationMessage: "This is the later SyncFuzz fork-observation step, so the earlier instruction about not creating directory-residue-fork-check.txt no longer applies. Without recreating or modifying branch-dir, use exactly one shell tool call to run: if [ -d branch-dir ]; then printf 'PRESENT_BRANCH_DIR\\n' > directory-residue-fork-check.txt; ls -ld branch-dir >> directory-residue-fork-check.txt; else printf 'MISSING_BRANCH_DIR\\n' > directory-residue-fork-check.txt; fi; cat directory-residue-fork-check.txt. After that, briefly explain whether branch-dir already existed in this fork.",
		},
		{
			TaskID:                  deleteResidueForkTargetTaskID,
			Description:             "fork from a pre-delete checkpoint and observe whether branch-delete-note.txt wrongly stays absent in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-delete checkpoint still sees a workspace file as absent even though the checkpoint predates deletion.",
			Prompt:                  "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create branch-delete-note.txt in the current working directory by running exactly:\n   printf '%s\\n' 'SYNCFUZZ_DELETE_RESIDUE_MARKER' > branch-delete-note.txt\n2. In a later shell step, confirm that branch-delete-note.txt exists and contains the marker by using a command in the style of:\n   ls -l branch-delete-note.txt && od -c branch-delete-note.txt\n3. In one later shell step, delete branch-delete-note.txt.\n4. In a later shell step, confirm that branch-delete-note.txt is absent by using a command in the style of:\n   ls branch-delete-note.txt 2>&1; echo \"exit_code=$?\"\n5. Do not create delete-residue-fork-check.txt during this initial branch. A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the deletion step.\n2. Create branch-delete-note.txt exactly once during the initial run.\n3. Delete branch-delete-note.txt exactly once during the initial run.\n4. Do not recreate branch-delete-note.txt after deleting it.\n5. Do not use `echo -n`, `cat`, `head`, or `tail` when checking branch-delete-note.txt.\n6. Do not store the marker in helper files, shell init files, `/tmp`, `/var/tmp`, `$HOME`, or other external paths.\n7. Leave the workspace ready for a later follow-up message that will only observe whether branch-delete-note.txt still exists from a forked checkpoint.\n8. After finishing, briefly explain that branch-delete-note.txt was deleted in the initial branch.",
			ExpectedFiles:           []string{targetDeleteResidueForkArtifact, langgraphForkArtifact},
			CheckpointSelector:      "before-file-delete",
			ForkVerificationMessage: "This is the later SyncFuzz fork-observation step, so the earlier instruction about not creating delete-residue-fork-check.txt no longer applies. Without recreating, deleting, or modifying branch-delete-note.txt, use exactly one shell tool call to run: if [ -f branch-delete-note.txt ]; then printf 'PRESENT_BRANCH_DELETE_NOTE\\n' > delete-residue-fork-check.txt; ls -l branch-delete-note.txt >> delete-residue-fork-check.txt; od -c branch-delete-note.txt >> delete-residue-fork-check.txt; else printf 'MISSING_BRANCH_DELETE_NOTE\\n' > delete-residue-fork-check.txt; fi; cat delete-residue-fork-check.txt. After that, briefly explain whether branch-delete-note.txt already existed in this fork.",
		},
		{
			TaskID:                  symlinkResidueForkTargetTaskID,
			Description:             "fork from a pre-symlink checkpoint and observe whether branch-link.txt still exists in the workspace",
			Objective:               "Observe whether LangGraph fork from a pre-symlink checkpoint still sees a workspace symlink that should have disappeared.",
			Prompt:                  "You are running inside a SyncFuzz experiment workspace with a persistent shell tool.\n\nPerform these steps using the shell tool:\n\n1. In one shell step, create a symbolic link named branch-link.txt in the current working directory that points exactly to target-prompt.txt.\n2. In a later shell step, print the result of readlink branch-link.txt to confirm that the symlink exists and points to target-prompt.txt.\n3. Do not create symlink-residue-fork-check.txt during this initial branch. A later SyncFuzz follow-up may explicitly ask you to create it as a witness artifact.\n4. Do not delete, rename, or modify branch-link.txt after creating it.\n\nImportant constraints:\n\n1. Use multiple shell calls so SyncFuzz can fork from before the symlink-creation step.\n2. Create branch-link.txt exactly once during the initial run.\n3. Do not replace branch-link.txt with a regular file.\n4. Do not store helper state in `/tmp`, `/var/tmp`, `$HOME`, shell init files, or other external paths.\n5. Leave the workspace ready for a later follow-up message that will only observe whether branch-link.txt still exists from a forked checkpoint.\n6. After finishing, briefly explain that branch-link.txt points to target-prompt.txt.",
			ExpectedFiles:           []string{targetSymlinkResidueForkArtifact, langgraphForkArtifact},
			CheckpointSelector:      "before-symlink-create",
			ForkVerificationMessage: "This is the later SyncFuzz fork-observation step, so the earlier instruction about not creating symlink-residue-fork-check.txt no longer applies. Without recreating or modifying branch-link.txt, use exactly one shell tool call to run: if [ -L branch-link.txt ]; then readlink branch-link.txt > symlink-residue-fork-check.txt; else printf 'MISSING_BRANCH_LINK\\n' > symlink-residue-fork-check.txt; fi; cat symlink-residue-fork-check.txt. After that, briefly explain whether branch-link.txt already existed in this fork.",
		},
	}
}

func workspaceResidueTaskSpecByID(taskID string) (workspaceResidueTaskSpec, bool) {
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
