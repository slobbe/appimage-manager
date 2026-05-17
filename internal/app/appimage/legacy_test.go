package appimage

func SetFilesystem(filesystem Filesystem) {
	defaultService.Filesystem = filesystem
}

func SetExtractor(extractor Extractor) {
	defaultService.Extractor = extractor
}

func SetDesktopEntryRewriter(rewriter DesktopEntryRewriter) {
	defaultService.DesktopEntryRewriter = rewriter
}

func SetPaths(paths Paths) {
	defaultService.Paths = paths
}
