# cmd/sb/Makefile — Build the sb CLI

.PHONY: build sync-skills clean

# Build the sb binary (syncs skills first)
build: sync-skills
	go build -o ../../bin/sb .

# Sync sb/ skill bundle → cmd/sb/skilldata/ for go:embed
sync-skills:
	@echo "Syncing sb/ → cmd/sb/skilldata/..."
	@rm -rf skilldata/
	@mkdir -p skilldata/skills/shen-backpressure skilldata/commands
	@cp ../../sb/skills/shen-backpressure/SKILL.md skilldata/skills/shen-backpressure/
	@cp ../../sb/commands/*.md skilldata/commands/
	@cp ../../sb/AGENT_PROMPT.md skilldata/
	@echo "  done"

clean:
	rm -f ../../bin/sb
	rm -rf skilldata/
