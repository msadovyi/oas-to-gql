const express = require('express')
const bodyParser = require('body-parser')
const app = express()


const jsonParser = bodyParser.json()
const urlencodedParser = bodyParser.urlencoded({ extended: false })

app.use(jsonParser);
app.use(urlencodedParser);

const pets = [{
  id: "1",
  name: "cat",
  tag: "cute"
  },
  {
    id: "2",
    name: "dog",
    tag: "gentle"
  },
  {
    id: "3",
    name: "wolf",
    tag: "dangerous"
  },
]

app.get('/pets', function (req, res) {
  const { tags, limit } = req.query
  let petsResponse = pets
  if (tags && tags.length) {
    petsResponse = pets.filter((p) => tags.includes(p.tag))
  }
  if (limit) {
    petsResponse = petsResponse.slice(0, limit) 
  }
  res.json(petsResponse)
})

app.get('/pets/:id', function (req, res) {
  pet = pets.find((p) => p.id === req.params.id)
  if (!pet) {
    return sendBadUserInput(res, { message: "Pet not found", "id": req.params.id });
  }
  res.json(pet)
})

app.put('/pets/:id', function (req, res) {
  const { body, params } = req
  if (!body || !body.name) {
    return sendBadUserInput(res, { message: "Pet should have name", body });
  }

  pet = pets.find((p) => p.id === params.id)
  if (!pet) {
    return sendBadUserInput(res, { message: "Pet not found", "id": req.params.id });
  }
  pet.name = body.name;
  pet.tag = body.tag || '';
  res.json(pet)
})

app.delete('/pets/:id', function (req, res) {
  const id = req.params.id;
  if (!pets[id]) {
    return sendBadUserInput(res, { message: "Pet not found", "id": req.params.id });
  }

  pets.splice(id, 1);
  res.statusCode = 204
  res.end();
})

app.post('/pets', function (req, res) {
  const { body } = req
  if (!body || !body.name) {
    return sendBadUserInput(res, { message: "Pet should have tag and name", body });
  }
  const pet = { ...body, id: pets.length + 1 };

  pets.push(pet);
  res.json(pet);
})

app.listen(3000, function () {
  console.log('Pet API listening http://localhost:3000')
})

function sendBadUserInput(res, data) {
  res.statusCode = 400;
  res.json({ error: "Bad Request", ...data })
}