package cli

import (
	"errors"
	"fmt"
	"regexp"
)

type Command struct {
	Arguments          []*Argument
	Options            []*Option
	Flags              []*Flag
	optionsByShortName map[string]*Option
	optionsByName      map[string]*Option
	flagsByShortName   map[string]*Flag
	flagsByName        map[string]*Flag
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
	Description  string
	FullName     string
	ShortName    string
	IsOptional   bool
	IsParsed     bool
	DefaultValue bool
	Value        bool
}

func NewCommand() *Command {
	return &Command{
		Arguments:          []*Argument{},
		Options:            []*Option{},
		Flags:              []*Flag{},
		optionsByShortName: map[string]*Option{},
		optionsByName:      map[string]*Option{},
		flagsByShortName:   map[string]*Flag{},
		flagsByName:        map[string]*Flag{},
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

func (command *Command) RegisterFlag(name string, shortName string, description string, isOptional bool, defaultValue bool) {
	flag := &Flag{
		Description:  description,
		FullName:     name,
		ShortName:    shortName,
		IsOptional:   isOptional,
		DefaultValue: defaultValue,
	}

	command.flagsByName[name] = flag
	command.flagsByShortName[shortName] = flag

	command.Flags = append(command.Flags, flag)
}

func (command *Command) Init(commandArguments *[]string) error {
	arguments := *commandArguments

	parsedArguments := 0
	curPos := 0
	for curPos < len(arguments) {
		curArg := arguments[curPos]

		optionNameExp := regexp.MustCompile("-([0-9a-zA-Z:]+)")
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

			flag, flagExist := command.flagsByName[optionName]
			flag, flagShortExist := command.flagsByShortName[optionName]
			if flagExist || flagShortExist {
				flag.Value = true
				flag.IsParsed = true
				curPos += 1
				continue
			}

			option, optionExist := command.optionsByName[optionName]
			if !optionExist {
				option, optionExist = command.optionsByShortName[optionName]
				if !optionExist {
					return errors.New(fmt.Sprintf("Found option name %s with value %s, but such option was not registered", curArg, nextArg))
				}
			}

			autoFlagExp := regexp.MustCompile("([0-9a-zA-Z]+):")
			if autoFlagExp.MatchString(optionName) {
				possibleFlagName := autoFlagExp.FindStringSubmatch(optionName)[1]
				flag, flagExist = command.flagsByName[possibleFlagName]
				flag, flagShortExist = command.flagsByShortName[possibleFlagName]
				if flagExist || flagShortExist {
					flag.Value = true
					flag.IsParsed = true
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

	for _, flag := range command.Flags {
		if !flag.IsOptional && !flag.IsParsed {
			return errors.New(fmt.Sprintf("Flag %s marked as required, but such flag was not found", flag.FullName))
		}
		if flag.IsOptional && !flag.IsParsed {
			flag.Value = flag.DefaultValue
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
	if len(command.Flags) > 0 {
		help += "Flags: \n"
		for _, flag := range command.Flags {
			shortName := flag.ShortName
			var defaultValue int
			if flag.DefaultValue {
				defaultValue = 1
			} else {
				defaultValue = 0
			}
			help += fmt.Sprintf("	-%s, -%s: %s, default value: %d\n", shortName, flag.FullName, flag.Description, defaultValue)
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

func (command *Command) GetFlag(name string) *Flag {
	flag, exist := command.flagsByShortName[name]
	if !exist {
		flag, exist = command.flagsByName[name]
		if !exist {
			return nil
		}
	}

	return flag
}
