package ui

// chooserCloseKeeps is the Aspects and Bodies choosers' one closing rule, on
// trial from 2026-10-22: a tick is in force the moment it is made (Space
// previews at once), and every closing key (Enter and Esc) leaves it in
// force. Set to false it restores the previous rule in full: Enter keeps,
// Esc puts back the selection the chooser opened with, and the bar says
// Apply and Cancel. The old tests that pin that rule set it false themselves.
var chooserCloseKeeps = true
