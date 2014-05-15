package cmd

import (
	"fmt"
	"github.com/smira/aptly/deb"
	"github.com/smira/commander"
	"github.com/smira/flag"
	"sort"
)

func aptlyRepoMoveCopyImport(cmd *commander.Command, args []string) error {
	var err error
	if len(args) < 3 {
		cmd.Usage()
		return commander.ErrCommandError
	}

	command := cmd.Name()

	dstRepo, err := context.CollectionFactory().LocalRepoCollection().ByName(args[1])
	if err != nil {
		return fmt.Errorf("unable to %s: %s", command, err)
	}

	err = context.CollectionFactory().LocalRepoCollection().LoadComplete(dstRepo)
	if err != nil {
		return fmt.Errorf("unable to %s: %s", command, err)
	}

	var (
		srcRefList *deb.PackageRefList
		srcRepo    *deb.LocalRepo
	)

	if command == "copy" || command == "move" {
		srcRepo, err = context.CollectionFactory().LocalRepoCollection().ByName(args[0])
		if err != nil {
			return fmt.Errorf("unable to %s: %s", command, err)
		}

		if srcRepo.UUID == dstRepo.UUID {
			return fmt.Errorf("unable to %s: source and destination are the same", command)
		}

		err = context.CollectionFactory().LocalRepoCollection().LoadComplete(srcRepo)
		if err != nil {
			return fmt.Errorf("unable to %s: %s", command, err)
		}

		srcRefList = srcRepo.RefList()
	} else if command == "import" {
		var srcRemoteRepo *deb.RemoteRepo

		srcRemoteRepo, err = context.CollectionFactory().RemoteRepoCollection().ByName(args[0])
		if err != nil {
			return fmt.Errorf("unable to %s: %s", command, err)
		}

		err = context.CollectionFactory().RemoteRepoCollection().LoadComplete(srcRemoteRepo)
		if err != nil {
			return fmt.Errorf("unable to %s: %s", command, err)
		}

		if srcRemoteRepo.RefList() == nil {
			return fmt.Errorf("unable to %s: mirror not updated", command)
		}

		srcRefList = srcRemoteRepo.RefList()
	} else {
		panic("unexpected command")
	}

	context.Progress().Printf("Loading packages...\n")

	dstList, err := deb.NewPackageListFromRefList(dstRepo.RefList(), context.CollectionFactory().PackageCollection(), context.Progress())
	if err != nil {
		return fmt.Errorf("unable to load packages: %s", err)
	}

	srcList, err := deb.NewPackageListFromRefList(srcRefList, context.CollectionFactory().PackageCollection(), context.Progress())
	if err != nil {
		return fmt.Errorf("unable to load packages: %s", err)
	}

	srcList.PrepareIndex()

	var architecturesList []string

	withDeps := context.flags.Lookup("with-deps").Value.Get().(bool)

	if withDeps {
		dstList.PrepareIndex()

		// Calculate architectures
		if len(context.ArchitecturesList()) > 0 {
			architecturesList = context.ArchitecturesList()
		} else {
			architecturesList = dstList.Architectures(false)
		}

		sort.Strings(architecturesList)

		if len(architecturesList) == 0 {
			return fmt.Errorf("unable to determine list of architectures, please specify explicitly")
		}
	}

	toProcess, err := srcList.Filter(args[2:], withDeps, dstList, context.DependencyOptions(), architecturesList)
	if err != nil {
		return fmt.Errorf("unable to %s: %s", command, err)
	}

	var verb string

	if command == "move" {
		verb = "moved"
	} else if command == "copy" {
		verb = "copied"
	} else if command == "import" {
		verb = "imported"
	}

	err = toProcess.ForEach(func(p *deb.Package) error {
		err = dstList.Add(p)
		if err != nil {
			return err
		}

		if command == "move" {
			srcList.Remove(p)
		}
		context.Progress().ColoredPrintf("@g[o]@| %s %s", p, verb)
		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to %s: %s", command, err)
	}

	if context.flags.Lookup("dry-run").Value.Get().(bool) {
		context.Progress().Printf("\nChanges not saved, as dry run has been requested.\n")
	} else {
		dstRepo.UpdateRefList(deb.NewPackageRefListFromPackageList(dstList))

		err = context.CollectionFactory().LocalRepoCollection().Update(dstRepo)
		if err != nil {
			return fmt.Errorf("unable to save: %s", err)
		}

		if command == "move" {
			srcRepo.UpdateRefList(deb.NewPackageRefListFromPackageList(srcList))

			err = context.CollectionFactory().LocalRepoCollection().Update(srcRepo)
			if err != nil {
				return fmt.Errorf("unable to save: %s", err)
			}
		}
	}

	return err
}

func makeCmdRepoMove() *commander.Command {
	cmd := &commander.Command{
		Run:       aptlyRepoMoveCopyImport,
		UsageLine: "move <src-name> <dst-name> <package-spec> ...",
		Short:     "move packages between local repositories",
		Long: `
Command move moves packages matching <package-spec> from local repo
<src-name> to local repo <dst-name>.

Example:

  $ aptly repo move testing stable 'myapp (=0.1.12)'
`,
		Flag: *flag.NewFlagSet("aptly-repo-move", flag.ExitOnError),
	}

	cmd.Flag.Bool("dry-run", false, "don't move, just show what would be moved")
	cmd.Flag.Bool("with-deps", false, "follow dependencies when processing package-spec")

	return cmd
}
