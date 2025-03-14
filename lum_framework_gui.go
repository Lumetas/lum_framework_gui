package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"bufio"
)

var bashCode string

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run server.go <static-files-dir>")
		return
	}

	staticDir := os.Args[1]

	// Чтение Bash-кода до EOF
	// fmt.Println("Введите Bash-код (завершите ввод EOF):")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "EOF" {
			break
		}
		bashCode += line + "\n"
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("Ошибка чтения ввода: %v\n", err)
		return
	}

	// Находим свободный порт
	port, err := findFreePort()
	if err != nil {
		fmt.Printf("Не удалось найти свободный порт: %v\n", err)
		return
	}

	// Запуск HTTP-сервера
	http.HandleFunc("/LUMFRAMEWORK", lumFrameworkHandler)
	http.HandleFunc("/execute", executeBashCode)
	http.Handle("/", http.FileServer(http.Dir(staticDir)))

	url := fmt.Sprintf("http://localhost:%d", port)
	// fmt.Printf("Сервер запущен на %s\n", url)

	// Запуск lum.gui.client
	cmd, err := startGUI(url)
	if err != nil {
		fmt.Printf("Не удалось запустить lum.gui.client: %v\n", err)
		return
	}

	// Ожидание завершения процесса lum.gui.client
	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("%v\n", err)
		} else {
			fmt.Println("")
		}
		os.Exit(0) // Завершаем программу
	}()

	// Запуск сервера
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		fmt.Printf("Ошибка сервера: %v\n", err)
	}
}

// Обработчик для /LUMFRAMEWORK
func lumFrameworkHandler(w http.ResponseWriter, r *http.Request) {
	jsCode := `
		const lum = {
			execute: async function(command) {
				const response = await fetch('/execute', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ code: command })
				});
				if (!response.ok) {
					throw new Error('Ошибка сервера: ' + response.statusText);
				}
				return await response.json();
			}
		};

		// Функция для выполнения Bash-функции через app_server
		async function executeAppServerFunction(id, event, data = null) {
			const command = "app_server." + id + ":" + event + (data ? " " + JSON.stringify(data) : "");
			try {
				const response = await lum.execute(command);
				eval(response.output); // Вывод результата в консоль
			} catch (error) {
				console.error("Ошибка выполнения команды " + command + ":", error);
			}
		}

		// Функция для добавления обработчиков событий
		function addEventListeners(element) {
			if (!element.id) return; // Пропускаем элементы без id

			// Обработчик клика
			if (element instanceof HTMLElement) {
				element.addEventListener('click', () => executeAppServerFunction(element.id, 'click'));
			}

			// Обработчик наведения (hover)
			if (element instanceof HTMLElement) {
				element.addEventListener('mouseover', () => executeAppServerFunction(element.id, 'hover'));
			}

			// Обработчик ввода (input)
			if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
				element.addEventListener('input', (e) => executeAppServerFunction(element.id, 'input', e.target.value));
			}

			// Добавьте другие события по необходимости
		}

		// Функция для обработки изменений DOM
		function observeDOM() {
			const observer = new MutationObserver((mutations) => {
				mutations.forEach((mutation) => {
					mutation.addedNodes.forEach((node) => {
						if (node.nodeType === Node.ELEMENT_NODE) {
							addEventListeners(node); // Добавляем обработчики для новых элементов
						}
					});
				});
			});

			// Начинаем наблюдение за изменениями DOM
			observer.observe(document.body, {
				childList: true,
				subtree: true
			});
		}

		// Инициализация
		document.addEventListener('DOMContentLoaded', () => {
			// Добавляем обработчики для существующих элементов
			document.querySelectorAll('[id]').forEach(addEventListeners);

			// Начинаем отслеживать изменения DOM
			observeDOM();
		});
	`
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(jsCode))
}

// Обработчик для выполнения Bash-кода
func executeBashCode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Объединяем сохранённый Bash-код с кодом от клиента
	fullCode := bashCode + "\n" + request.Code

	// Выполняем в Bash
	cmd := exec.Command("bash", "-c", fullCode)
	output, err := cmd.CombinedOutput()
	if err != nil {
		response := map[string]string{"error": fmt.Sprintf("Error executing Bash code: %s", err)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Возвращаем результат
	response := map[string]string{"output": string(output)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Функция для поиска свободного порта
func findFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

// Функция для запуска lum.gui.client
func startGUI(url string) (*exec.Cmd, error) {
	// Получаем текущую директорию
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить текущую директорию: %v", err)
	}

	// Формируем полный путь к бинарнику
	binaryPath := filepath.Join(dir, "lib", "lum.gui.client")

	// Проверяем, существует ли бинарник
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("lum.gui.client не найден в %s", binaryPath)
	}

	// Запускаем бинарник
	cmd := exec.Command(binaryPath, url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}
