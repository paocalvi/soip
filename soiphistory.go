package main

import (
	"fmt"
)

type CommandHistory struct {
	commands []string
	position int
}

func (ch *CommandHistory) addToHistory(newone string) {
	ch.commands = append(ch.commands, newone)
	ch.position = len(ch.commands)
}

func (ch *CommandHistory) goBack() (toret string) {
	fmt.Println("BACK1", ch.asString())
	if ch.position == 0 {
		return ""
	}
	toret = ch.commands[ch.position-1]
	ch.position--
	fmt.Println("BACK2", ch.asString())
	return
}

func (ch *CommandHistory) goFwd() (toret string) {
	fmt.Println("NEXT1", ch.asString())
	if ch.position >= len(ch.commands)-1 {
		return ""
	}
	toret = ch.commands[ch.position+1]
	ch.position++
	fmt.Println("NEXT2", ch.asString())
	return
}

func (ch *CommandHistory) asString() string {
	toret := fmt.Sprintf("%d:", ch.position)
	for _, pezzo := range ch.commands {
		toret = toret + " " + pezzo
	}
	return toret
}
