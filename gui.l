#!/bin/bash
function lum.gui.start {
	class_prepare $1 app_server "source modules.l" | ./lib/lum_framework_gui $2;
}

