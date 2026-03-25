package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/memory"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage synced Memory.md fragments",
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memory fragments in the sync repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncMemDir := paths.SyncMemoryDir()
		m, err := memory.ReadManifest(syncMemDir)
		if err != nil {
			return err
		}
		if len(m.Order) == 0 {
			fmt.Println("No memory fragments in sync repo.")
			return nil
		}

		// Read config to check which are included.
		cfgData, err := os.ReadFile(paths.ConfigFile())
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return err
		}
		includeSet := make(map[string]bool, len(cfg.Memory.Include))
		for _, name := range cfg.Memory.Include {
			includeSet[name] = true
		}

		for _, name := range m.Order {
			meta := m.Fragments[name]
			status := "  "
			if includeSet[name] {
				status = "* "
			}
			fmt.Printf("%s%-30s [%s] %s\n", status, name, meta.Type, meta.Description)
		}
		return nil
	},
}

var memoryImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import memory files from Claude Code into the sync repo",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncMemDir := paths.SyncMemoryDir()
		result, err := memory.ImportFromDir(paths.ClaudeMemoryDir(), syncMemDir)
		if err != nil {
			return err
		}
		if len(result.Imported) == 0 {
			fmt.Println("No new memory files found to import.")
			return nil
		}
		fmt.Printf("Imported %d memory file(s):\n", len(result.Imported))
		for _, name := range result.Imported {
			fmt.Printf("  + %s\n", name)
		}
		return nil
	},
}

var memoryAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Import a specific memory file and add to sync",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		fm, err := memory.ParseFrontmatter(string(content))
		if err != nil {
			return fmt.Errorf("parsing frontmatter: %w", err)
		}
		if fm.Name == "" {
			return fmt.Errorf("frontmatter is missing required 'name' field")
		}

		syncMemDir := paths.SyncMemoryDir()
		if err := os.MkdirAll(syncMemDir, 0755); err != nil {
			return fmt.Errorf("creating sync memory dir: %w", err)
		}

		slug := memory.SlugifyName(fm.Name)

		// Read manifest to check for existing fragment.
		m, err := memory.ReadManifest(syncMemDir)
		if err != nil {
			return err
		}
		if _, exists := m.Fragments[slug]; exists {
			return fmt.Errorf("fragment %q already exists in sync repo; use a different name or remove it first", slug)
		}
		if err := memory.WriteFragment(syncMemDir, slug, string(content)); err != nil {
			return err
		}

		// Update manifest.
		m.Fragments[slug] = memory.FragmentMeta{
			Name:        fm.Name,
			Description: fm.Description,
			Type:        fm.Type,
			Level:       "user",
			ContentHash: memory.ContentHash(string(content)),
		}
		m.Order = append(m.Order, slug)
		if err := memory.WriteManifest(syncMemDir, m); err != nil {
			return err
		}

		// Update config.
		cfgData, err := os.ReadFile(paths.ConfigFile())
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return err
		}
		cfg.Memory.Include = append(cfg.Memory.Include, slug)
		newData, err := config.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(paths.ConfigFile(), newData, 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		fmt.Printf("Added %s (%s)\n", slug, fm.Type)
		return nil
	},
}

var memoryRemovePurge bool

var memoryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a memory fragment from sync",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// Remove from config include list.
		cfgData, err := os.ReadFile(paths.ConfigFile())
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}
		cfg, err := config.Parse(cfgData)
		if err != nil {
			return err
		}
		var filtered []string
		for _, inc := range cfg.Memory.Include {
			if inc != name {
				filtered = append(filtered, inc)
			}
		}
		cfg.Memory.Include = filtered
		newData, err := config.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(paths.ConfigFile(), newData, 0644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		if memoryRemovePurge {
			syncMemDir := paths.SyncMemoryDir()
			fragPath := filepath.Join(syncMemDir, name+".md")
			if err := os.Remove(fragPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("deleting fragment file: %w", err)
			}
			m, err := memory.ReadManifest(syncMemDir)
			if err != nil {
				return fmt.Errorf("reading manifest for purge: %w", err)
			}
			delete(m.Fragments, name)
			var newOrder []string
			for _, n := range m.Order {
				if n != name {
					newOrder = append(newOrder, n)
				}
			}
			m.Order = newOrder
			if err := memory.WriteManifest(syncMemDir, m); err != nil {
				return fmt.Errorf("writing manifest after purge: %w", err)
			}
		}

		fmt.Printf("Removed %s from sync\n", name)
		return nil
	},
}

func init() {
	memoryRemoveCmd.Flags().BoolVar(&memoryRemovePurge, "purge", false, "Also delete the fragment file")

	memoryCmd.AddCommand(memoryListCmd)
	memoryCmd.AddCommand(memoryImportCmd)
	memoryCmd.AddCommand(memoryAddCmd)
	memoryCmd.AddCommand(memoryRemoveCmd)

	rootCmd.AddCommand(memoryCmd)
}
