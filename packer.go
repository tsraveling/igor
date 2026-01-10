package main

// func walkFilesCmd(path string) tea.Cmd {
// 	return func() tea.Msg {
// 		filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
// 			if err != nil {
// 				return err
// 			}
// 			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
// 				relPath, _ := filepath.Rel(path, p)
// 				f, err := os.Open(p)
// 				if err != nil {
// 					return nil
// 				}
// 				defer f.Close()
//
// 				img, _, err := image.DecodeConfig(f)
// 				if err != nil {
// 					return nil
// 				}
//
// 				prg.Send(preparedFileMsg{
// 					file: imageResult{
// 						path:     relPath,
// 						filename: d.Name(),
// 						w:        img.Width,
// 						h:        img.Height,
// 					}})
// 			}
// 			return nil
// 		})
// 		return prepareCompleteMsg{}
// 	}
// }
