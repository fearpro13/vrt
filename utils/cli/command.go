package cli

import (
	"errors"
	"fmt"
	"regexp"
)

type Command struct {
	Arguments          []*Argument
	Options            []*Option
	optionsByShortName map[string]*Option
	optionsByName      map[string]*Option
}

type Argument struct {
	Description string
	Position    int
	Value       string
}

type Option struct {
	Description  string
	FullName     string
	ShortName    string
	IsOptional   bool
	IsParsed     bool
	DefaultValue string
	Value        string
}

type Flag struct {
	Option
	DefaultValue bool
	Value        bool
}

func NewCommand() *Command {
	return &Command{
		Arguments:          []*Argument{},
		Options:            []*Option{},
		optionsByShortName: map[string]*Option{},
		optionsByName:      map[string]*Option{},
	}
}

func (command *Command) RegisterArgument(description string) {
	argument := &Argument{Description: description}
	command.Arguments = append(command.Arguments, argument)
}

func (command *Command) RegisterOption(name string, shortName string, description string, isOptional bool, defaultValue string) {
	option := &Option{
		Description:  description,
		FullName:     name,
		ShortName:    shortName,
		IsOptional:   isOptional,
		DefaultValue: defaultValue,
	}

	command.optionsByName[name] = option
	command.optionsByShortName[shortName] = option

	command.Options = append(command.Options, option)
}

func (command *Command) Init(commandArguments *[]string) error {
	arguments := *commandArguments
	//if len(arguments) < len(command.Arguments) {
	//	return errors.New(fmt.Sprintf("Not enough arguments, expected %d arguments and %d options", len(command.Arguments), len(command.Options)))
	//}
	//
	//for i := 0; i <= len(command.Arguments); i++ {
	//	command.ParsedArguments = append(command.ParsedArguments, arguments[i])
	//}

	parsedArguments := 0
	curPos := 0
	for curPos < len(arguments) {
		curArg := arguments[curPos]

		optionNameExp := regexp.MustCompile("-(\\w+)")
		if !optionNameExp.MatchString(curArg) {
			if parsedArguments < len(command.Arguments) {
				command.Arguments[parsedArguments].Value = curArg
				parsedArguments++
				curPos++
			} else {
				return errors.New(fmt.Sprintf("Wrong option %s, most probably you are missing prefix '-'", curArg))
			}
		} else {
			if curPos+1 > len(arguments)-1 {
				return errors.New(fmt.Sprintf("Found option name %s, but without option value", curArg))
			}

			nextArg := arguments[curPos+1]

			optionName := optionNameExp.FindStringSubmatch(curArg)[1]

			option, optionExist := command.optionsByName[optionName]
			if !optionExist {
				option, optionExist = command.optionsByShortName[optionName]
				if !optionExist {
					return errors.New(fmt.Sprintf("Found option name %s with value %s, but such option was not registered", curArg, nextArg))
				}
			}

			option.Value = nextArg
			option.IsParsed = true
			curPos += 2
		}
	}

	if parsedArguments < len(command.Arguments) {
		return errors.New(fmt.Sprintf("Not enough arguments, expected %d, but got %d", len(command.Arguments), parsedArguments))
	}

	for _, option := range command.Options {
		if !option.IsOptional && !option.IsParsed {
			return errors.New(fmt.Sprintf("Option %s marked as required, but no value was passed for such option", option.FullName))
		}
		if option.IsOptional && !option.IsParsed {
			option.Value = option.DefaultValue
		}
	}

	return nil
}

func (command *Command) PrintHelp() string {
	help := "Usage: \n"
	if len(command.Arguments) > 0 {
		help += "Arguments:\n"
		for index, argument := range command.Arguments {
			help += fmt.Sprintf("	%d - %s\n", index+1, argument.Description)
		}
	}
	if len(command.Options) > 0 {
		help += "Options: \n"
		for _, option := range command.Options {
			shortName := option.ShortName
			defaultValue := option.DefaultValue
			help += fmt.Sprintf("	-%s, -%s: %s, default value: %s\n", shortName, option.FullName, option.Description, defaultValue)
		}
	}

	return help
}

func (command *Command) GetArgument(position int) *Argument {
	return command.Arguments[position]
}

func (command *Command) GetOption(name string) *Option {
	option, exist := command.optionsByShortName[name]
	if !exist {
		option, exist = command.optionsByName[name]
		if !exist {
			return nil
		}
	}

	return option
}
