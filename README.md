# Containerization & Orchestration Labs

Учебный репозиторий команды из 3 студентов для выполнения лабораторных работ по контейнеризации и оркестрации.

## 👥 Команда

| Участник  | GitHub                                   | Telegram |
| --------- | ---------------------------------------- | ---------------------------------------- |
| Student 1 | [Kailber](https://github.com/Kailber)   | [@beverss](https://t.me/beverss)         |
| Student 2 | [12262004-m](https://github.com/12262004-m) | [@MpAsSgHA](https://t.me/MpAsSgHA) |
| Student 3 | [Diminasss](https://github.com/Diminasss) | [@diminass](https://t.me/diminass) |

## 📚 Лабораторные работы

| №  | Тема                                               | Статус         |
| -- | -------------------------------------------------- | -------------- |
| 01 | Свой Docker                                        | 🟢 Done        |
| 02 | Мониторинг сервиса: метрики, логи, трейсы          | 🟢 Done        |
| 03 | Собери платформу для shop                          | 🟢 Done        |
| 04 | SLA критического пути под перегрузкой              | 🟢 Done        |
| 05 | Zero-trust сеть с доказательством                  | 🟢 Done        |
| 06 | Автопилот релиза: катит хорошее, откатывает плохое | 🐸 Frog Skip   |
| 08 | Автоматизация devops-процесса с n8n + AI           | 🟢 Done        |

### Статусы

* 🟢 Done
* 🟡 In progress
* ⚪ Not started
* 🐸 Frog Skip

## 🛠️ Технологии

* Docker
* Kubernetes
* Git / GitHub
* Go

## 📁 Структура проекта

```text
.
├── lab1/
├── lab2/
├── lab3/
├── lab4/
├── lab5/
├── lab6/
├── lab8/
└── README.md
```

Каждая лабораторная работа находится в отдельной директории и содержит собственный `README.md` с инструкцией по запуску и описанием решения.

## 🚀 Как работать с репозиторием

Клонировать репозиторий:

```bash
git clone https://github.com/Kailber/containerization-and-orchestration-labs.git
cd containerization-and-orchestration-labs
```

Перед началом работы:

```bash
git pull
```

После внесения изменений:

```bash
git add .
git commit -m "lab1: add Dockerfile"
git push
```

## 🌿 Git workflow

Для каждой лабораторной рекомендуется создавать отдельную ветку:

```text
main
 ├── lab1
 ├── lab2
 ├── lab3
 └── lab4
```

Пример:

```bash
git checkout -b lab1
```

После завершения работы создаётся Pull Request в `main`.

## 📝 Правила коммитов

Рекомендуемый формат:

```text
lab1: add Dockerfile
lab1: configure container networking
lab2: add docker compose
lab3: fix healthcheck
lab4: add Kubernetes deployment
```

## 🔀 Pull Requests

Перед merge в `main` желательно:

1. Убедиться, что лабораторная запускается.
2. Проверить README лабораторной.
3. Удалить временные файлы и секреты.
4. Убедиться, что `git status` не содержит лишних файлов.

## 🔐 Secrets

Не коммитить в репозиторий:

* `.env`
* пароли
* API keys
* SSH keys
* kubeconfig
* credentials
* приватные сертификаты

## 📖 Лабораторные

Подробная информация о каждой лабораторной находится в соответствующей директории:

* [Lab 1](./lab1/)
* [Lab 2](./lab2/)
* [Lab 3](./lab3/)
* [Lab 4](./lab4/)
* [Lab 5](./lab5/)
* [Lab 6](./lab6/)
* [Lab 7](./lab7/)
* [Lab 8](./lab8/)
