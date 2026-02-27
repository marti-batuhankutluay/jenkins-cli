.PHONY: build install uninstall

BINARY := jenkins-cli
INSTALL_DIR := $(HOME)/.local/bin

build:
	go build -o $(BINARY) .

install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed: $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH."
	@echo "  Add to ~/.zshrc:  export PATH=\"\$$HOME/.local/bin:\$$PATH\""

uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "Removed $(INSTALL_DIR)/$(BINARY)"
